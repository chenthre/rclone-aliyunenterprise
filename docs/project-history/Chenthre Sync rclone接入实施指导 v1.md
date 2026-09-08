# Chenthre Sync：基于 rclone 的阿里云盘企业版同步后端实施指导 v1

> 适用项目：当前 `notes-sync` / Chenthre 项目  
> 当前阶段：技术方案收敛与 Ubuntu PoC  
> 核心目标：**复用成熟的 rclone/bisync 同步能力，只自行实现阿里云盘企业版的 rclone Backend 适配层。**
>
> 本文是实施指导，不是对话记录。执行 Agent 应以本文中的“目标、约束、验收条件”为准；对尚未验证的内容必须通过实验确认，不得当作既成事实。

---

## 1. 任务目标

### 1.1 最终产品背景

Chenthre 的长期目标是一个 Android 优先的个人 AI 增强应用，未来可能覆盖：

- 笔记；
- 日程；
- Todo；
- random idea / quick capture；
- AI 辅助整理、检索、规划和执行。

当前第一个基础能力是**跨设备文件同步**，主要服务于上述个人信息数据，并首先兼容类似 Obsidian Vault 的普通目录树。

### 1.2 当前任务目标

本阶段**不从零开发一个新的 Sync Core 算法**。

当前目标是验证并实现以下技术路线：

```text
Chenthre App
    │
    ▼
Thin SyncService facade
    │
    ▼
rclone / bisync
    │
    ▼
rclone fs.Fs abstraction
    │
    ▼
Aliyun Drive Enterprise Backend（自研）
    │
    ▼
阿里云盘企业版 + 企业版 API Key
```

核心思想：

1. **同步算法尽量复用成熟 OSS：rclone bisync。**
2. **项目特有代码只处理阿里云盘企业版的兼容层。**
3. Chenthre 上层只依赖一个很薄的 `SyncService`，不直接依赖 rclone 或阿里 API。
4. 将来如果切换为 iCloud、S3、WebDAV 或其他远端，优先复用 rclone 已有 Backend；不应重写 Chenthre 的业务层。
5. 当前只证明并实现 Ubuntu 端 PoC；Android 集成在此之后。

---

## 2. 期望结果

完成本阶段后，应得到一个可重复验证的 Ubuntu PoC：

```text
device-a-vault/
        ↕
   rclone bisync
        ↕
自研 aliyunenterprise backend
        ↕
阿里云盘企业版 Sync 空间
        ↕
   rclone bisync
        ↕
device-b-vault/
```

需要达到以下可观察结果：

- 一个**不修改 rclone 主仓库源码**的独立 Aliyun Enterprise backend；
- 一个固定 rclone 版本构建出的 `rclone-chenthre` 或等价测试二进制；
- Backend 可以完成 rclone 所需的基本文件操作；
- 阿里云 `file/list` 子目录异常和删除 API 不可用，对 rclone 上层透明；
- `rclone bisync` 可以在两个模拟设备目录之间通过真实阿里云盘执行双向同步；
- 新建、修改、重命名/移动、逻辑删除和普通冲突场景不会静默丢失数据；
- 中断或 API 暂时失败时应**失败关闭（fail closed）**，而不是把不确定状态解释为删除；
- 测试和调试不要求真实 Obsidian 主 Vault，全部使用隔离测试数据；
- API Key 不进入代码、日志、测试快照或 Git 历史。

如果 PoC 达到上述状态，则可正式采用：

> `rclone bisync + Aliyun Enterprise Backend + Chenthre SyncService facade`

作为 Chenthre 文件同步第一版技术路线。

---

## 3. 背景与已确认事实

### 3.1 用户当前拥有的服务

必须明确：

> **当前只有阿里云盘企业版（Aliyun Drive Enterprise）订阅，没有 PDS Developer Edition。**

不要在实现中假设存在开发者版 PDS Domain、RAM AccessKey、OAuth 或 `*.api.aliyunpds.com` 环境。

### 3.2 企业版 API Key 已验证

现有实验已确认：

```http
Authorization: Bearer <uk-...>
```

可以直接调用企业版 REST 网关：

```text
https://<enterprise-domain-id>.api.aliyunfile.com/v2/*
```

已真实验证可用的能力包括：

- 枚举可访问 Drive；
- 根目录 List；
- Create File / Folder；
- Upload / multipart complete；
- Download；
- Get metadata；
- Search；
- Overwrite；
- Rename；
- Move；
- Copy；
- SHA1 `content_hash`；
- `file_id` 稳定标识。

配置值（Domain ID、Drive ID、API Key 等）必须从环境或配置文件读取，不应硬编码到 Backend 源码。

### 3.3 两个已确认的企业版限制

#### A. 子目录 `file/list` 异常

真实环境表现：

```text
file/list(root)        → 正常
file/list(subfolder)   → items:[]
file/list_delta        → items:[]
```

而同一对象可以通过：

```text
search-file
get-file
create
upload
move
copy
```

正常访问。

已经有可工作的 Python 回退实现：

`pds_list_children.py`

其策略：

```text
ListFile
   │
   ├─ root 或有结果 → 使用 List
   │
   └─ 子目录空
         ↓
     SearchFile(parent_file_id=<parent>, recursive=false)
         ↓
     GetFile(file_id) 逐项复核真实 parent
```

Search 为最终一致索引，因此可能短暂漏掉刚创建或刚变更的对象。

#### B. API Key 不具有真正删除能力

以下接口已确认不可向企业版 API Key 开通：

```text
DeleteFile
TrashFile / recyclebin
```

表现为：

```text
403 CheckRouterAccessFailed
```

因此本 Backend 不得依赖云端真正 DELETE。

---

## 4. 关键技术决策

### 4.1 使用 rclone，而不是自研 Sync Core

本阶段不自行实现：

- version vector；
- 双向差异比较算法；
- 同步 baseline 数据库；
- tombstone replication；
- conflict resolver；
- retry/recovery engine；
- 自定义 change log protocol。

原因不是这些设计错误，而是 rclone/bisync 已经提供成熟实现，而当前个人使用场景没有足够理由承担重复实现和长期维护成本。

### 4.2 rclone 是实现细节，不是 Chenthre 产品接口

Chenthre 上层未来只应依赖类似：

```kotlin
interface SyncService {
    suspend fun sync(): SyncResult
    suspend fun dryRun(): SyncResult
    suspend fun cancel()
    fun observeStatus(): Flow<SyncStatus>
}
```

当前 PoC 不要求正式实现 Android/Kotlin 接口，但代码组织不得让未来业务层必须理解：

- rclone CLI 参数；
- Aliyun `file_id`；
- API Key；
- Aliyun Search/List workaround。

### 4.3 Backend 采用 out-of-tree 模式

优先采用 rclone 官方支持的 **out-of-tree backend** 模式。

开发阶段推荐：

- 独立 Go module / repo；
- 依赖一个**固定版本** rclone；
- 自定义构建入口 import 官方 rclone + `aliyunenterprise` backend；
- 输出独立 `rclone-chenthre` 测试二进制。

不推荐长期维护 rclone fork。

Linux Go plugin 可以作为开发调试手段，但**不要将 plugin 作为最终架构基础**：

- Go plugin 目前主要适用于 Linux/macOS；
- plugin 必须与宿主 rclone 使用完全匹配的版本；
- 未来 Android 需要把 Backend 编译进 librclone/AAR。

因此跨平台可维护方案优先是：

> **compile-time out-of-tree backend**

而不是运行时 `.so` plugin。

### 4.4 固定 rclone 版本

不要依赖 `master`。

在 PoC 开始时：

1. 选择一个明确的 rclone release；
2. 在 `go.mod` 中 pin；
3. 文档记录版本；
4. 所有 contract test 和 bisync test 针对该版本运行；
5. 升级 rclone 必须重新跑 Backend contract + bisync regression suite。

### 4.5 Backend 隐藏 Aliyun 缺陷

Sync/bisync 层不允许出现：

```text
if provider == aliyun ...
```

所有特殊行为只存在于：

```text
AliyunEnterpriseBackend
```

包括：

- List → Search fallback；
- persistent object catalog；
- GetFile 校验；
- hidden trash；
- API Key Bearer；
- `file_id` 缓存；
- provider-specific retry。

---

## 5. rclone 采用理由与边界

### 5.1 为什么选择 rclone/bisync

rclone 的核心设计本身就是：

```text
统一 Fs/Object abstraction
        ↓
大量不同远端 Backend
```

`bisync` 已实现：

- 双向差异同步；
- 上一轮 listing/baseline；
- 新建/修改/删除判断；
- 冲突处理；
- checksum；
- retry；
- interrupted-run recovery；
- safety checks；
- dry-run；
- filtering；
- lock/workdir；
- 多种云存储支持。

因此当前项目只需要解决真正特殊的部分：

> **Aliyun Drive Enterprise Backend compatibility**

### 5.2 bisync 不是“无风险黑盒”

rclone 官方仍将 bisync 定义为 advanced command，并明确要求谨慎使用。

因此：

- 不能因为 rclone 成熟就跳过自己的回归测试；
- 不允许直接用真实主 Vault 作为第一批测试数据；
- 首次 `--resync` 必须先 dry-run；
- 不允许每一轮都使用 `--resync`；
- 必须显式配置 safety options；
- Chenthre 外层仍建议提供单实例锁，确保同一个 workspace 同时只有一个同步任务。

---

## 6. Aliyun Enterprise Backend 设计

### 6.1 建议内部结构

```text
aliyunenterprise/
├── backend.go
├── object.go
├── client.go
├── config.go
├── listing.go
├── catalog.go
├── trash.go
├── consistency.go
├── errors.go
└── *_test.go
```

职责建议：

```text
Fs / Object
      │
      ▼
Provider semantics
      │
      ├── REST Client
      ├── Path ↔ file_id Catalog
      ├── Directory Enumerator
      ├── Consistency Guard
      └── Hidden Trash
              │
              ▼
          Aliyun REST
```

### 6.2 REST Client

只负责协议：

- Base URL；
- Bearer Authentication；
- JSON encode/decode；
- HTTP status → typed error；
- timeout；
- retryable / non-retryable 分类；
- multipart upload；
- download URL；
- request logging（必须脱敏）。

Client 不负责：

- path resolution policy；
- rclone listing semantics；
- bisync logic。

### 6.3 Hash

Aliyun 已真实返回：

```text
content_hash = SHA1
```

Backend 应向 rclone 声明支持 SHA-1，并将 provider hash 映射到 rclone Hash API。

当前 bisync PoC 优先使用：

```text
--compare size,checksum
```

而不是依赖 modtime 作为主要变化依据。

`ModTime()` 仍按 rclone Backend contract 提供远端时间，但 correctness 不应建立在修改时间强一致上。

---

## 7. `List()` 是本 PoC 的核心风险点

### 7.1 rclone 需要的语义

对于给定目录：

```go
Fs.List(ctx, dir)
```

上层必须能够把其结果当作**该目录当前直属对象集合**使用。

如果 Backend 因 Search 索引暂时为空而错误告诉 bisync：

```text
“一个以前存在的文件不存在了”
```

bisync 可能将其解释为删除并传播到另一端。

这是当前最危险的数据损失路径。

### 7.2 基础策略

第一版：

```text
ListChildren(parent)
    │
    ▼
Native ListFile
    │
    ├─ root → 使用结果
    ├─ non-empty → 使用结果
    │
    └─ 子目录返回空
            ↓
SearchFile(
    parent_file_id = parent,
    recursive = false
)
            ↓
marker 全分页
            ↓
GetFile(file_id) 复核
            ↓
按真实 parent 过滤
```

### 7.3 Persistent Known-Object Catalog

仅依赖 Search fallback 不够安全。

Backend 应维护一个本地持久化 catalog，例如：

```text
provider_object
────────────────────────────
drive_id
file_id
parent_file_id
name
remote_path
size
sha1
modified_at
last_seen_at
```

具体存储可以用：

- SQLite；
- BoltDB；
- 小型嵌入式 KV；

由实现 Agent选择，优先简单和可靠。

它只属于 provider cache，不属于 Chenthre 业务数据。

### 7.4 Search miss 的处理原则

**硬性 invariant：**

> `Search absence != confirmed deletion`

对于 Search 当前未返回、但 catalog 已知的对象：

```text
known file_id
     ↓
GetFile(file_id)
     │
     ├── exists，parent 未变
     │       → 将对象补回当前 List
     │
     ├── exists，parent 已变
     │       → 更新 catalog，不属于原目录
     │
     └── authoritative not-found
             → 才认为该对象从该 provider 消失
```

由于企业版 API Key 本身不能删除，正常情况下 `GetFile` 的真正 404 应非常少见；但用户可能在 Web/官方客户端手动删除，所以仍需处理。

### 7.5 新对象 Search 延迟

如果一个**从未进入 catalog 的新对象**尚未被 Search 索引：

```text
这轮同步暂时发现不了
```

是允许的。

结果应该是：

> delayed discovery

而不是：

> destructive misclassification

后续同步轮次索引收敛后再发现。

### 7.6 Fail Closed

如果 Backend 无法判断目录 listing 是否可信，例如：

- Search API 连续失败；
- GetFile 大量异常；
- pagination 不完整；
- authentication 异常；
- 429/5xx 超过 retry budget；

则：

```text
List() 返回 error
```

让 bisync abort。

**严禁**为了“让同步继续”而返回一个可能不完整的空/半空 listing。

---

## 8. 删除语义：隐藏 Trash

### 8.1 rclone 对上层呈现删除

rclone 的 `Object.Remove()` / `Fs.Rmdir()` 在我们的 Backend 中实现为：

```text
MoveFile / MoveFolder
       ↓
provider-private hidden trash
```

例如逻辑 namespace：

```text
_chenthre_rclone_internal/
    trash/
```

该目录：

- 永远不应由普通 `List()` 返回；
- 不应被 bisync 视为用户数据；
- 名称应明确保留；
- 与现有测试 `_trash_chenthre_sync` 可以共存或在 PoC 时统一迁移，但不要把旧 `file_id` 硬编码在代码中。

### 8.2 Trash 对象命名

为避免同名冲突，不应简单：

```text
trash/foo.md
```

建议生成唯一对象，例如：

```text
trash/<timestamp>-<file_id>-<escaped-original-name>
```

或按 `file_id` 建目录。

需要记录足够元数据用于 debug / 人工恢复。

### 8.3 不要求物理垃圾回收

当前阶段：

```text
physical delete / GC
```

属于非目标。

管理员可在 Web 中手工清理 provider-private trash。

---

## 9. 第一版 Backend 能力范围

### 9.1 必须实现

以 rclone 当前接口和 bisync 实际需求为准，至少覆盖：

```text
Fs:
- List
- NewObject
- Put
- Mkdir
- Rmdir

Object:
- Open
- Update
- Remove
- Size
- ModTime
- Hash
```

以及所有相应的基础 metadata / info contract。

**注意：最终应以所 pin rclone 版本的接口定义和 fstest 结果为准，不能仅按本文的伪接口机械实现。**

### 9.2 可以实现的优化

Aliyun 已验证支持：

```text
server-side Move
server-side Copy
```

如果实现成本合理，可暴露给 rclone optional Features。

### 9.3 当前明确不实现

除非 Backend contract 强制要求，否则不要在第一版实现：

- `ListR`；
- ChangeNotify；
- Purge；
- PublicLink；
- About；
- metadata 扩展；
- provider-side version history；
- provider delta cursor；
- true Delete。

特别是 `ListR`：

当前 Search 的最终一致性尚不足以把它作为“可靠递归 listing”暴露给 rclone。

如果不实现，保持 Feature 为 nil，让 rclone 退回普通目录遍历。

---

## 10. 目录和代码组织建议

基于当前已有 `notes-sync` 项目，不要求大规模重构。

建议逐渐整理为：

```text
notes-sync/
├── docs/
│   ├── 当前实施指导
│   ├── 已有验证结果
│   └── Aliyun provider quirks
│
├── tools/
│   ├── pds_list_children.py
│   ├── sync-delete.sh
│   ├── recheck.py
│   └── 其他现有 probe
│
├── rclone-aliyunenterprise/
│   ├── go.mod
│   ├── backend/
│   │   └── aliyunenterprise/
│   └── cmd/
│       └── rclone-chenthre/
│
└── testdata/
    ├── device-a/
    └── device-b/
```

已有 Python/Shell 工具不要删除。

它们的定位改为：

> Aliyun Backend 行为基准、回归诊断工具和 REST reference implementation。

在 Go Backend 行为不确定时，可以拿它们对照真实服务端结果。

---

## 11. Secret 与配置约束

### 硬性要求

不得：

- 将 API Key commit；
- 把 API Key 写死进 Go；
- 把 API Key 打印进测试输出；
- 把 Authorization header 打进 debug log；
- 将真实 Key 放入 README；
- 将 `.env` 纳入版本管理。

推荐：

```text
.env
RCLONE config
OS key storage（未来）
```

PoC 可从环境变量读取：

```text
ALIYUN_ENTERPRISE_API_KEY
ALIYUN_ENTERPRISE_DOMAIN_ID
ALIYUN_ENTERPRISE_DRIVE_ID
```

具体名字可调整，但配置必须可注入。

错误日志只允许：

```text
Authorization: Bearer ***REDACTED***
```

---

## 12. 实施范围

本轮需要覆盖：

1. rclone out-of-tree Backend 工程骨架；
2. 企业版 REST Client；
3. Backend basic filesystem semantics；
4. List/Search/Get consistency guard；
5. persistent known-object catalog；
6. hidden-trash Remove/Rmdir；
7. SHA1 mapping；
8. rclone Backend integration/contract testing；
9. Ubuntu 两模拟设备 bisync PoC；
10. failure / conflict / eventual-consistency 测试；
11. 文档化 provider quirks；
12. 形成是否正式采用 rclone 的技术结论。

---

## 13. 非目标

当前明确**不做**：

- Android UI；
- Android SAF；
- WorkManager 后台调度；
- AI 功能；
- 日程/Todo 业务模型；
- Obsidian 插件；
- 真实主 Vault 上线；
- iOS/iCloud Adapter；
- 自研 Sync protocol；
- CRDT；
- P2P；
- 实时同步；
- 云端真正 Delete；
- blob GC；
- 块级 delta；
- E2EE；
- provider upstream 到 rclone 官方仓库。

未来可做，不应影响当前 PoC 收敛。

---

## 14. 实施计划

### P0 — 工程与基线固定

目标：

- 选择并 pin rclone release；
- 建立 out-of-tree backend；
- 可以编译 `rclone-chenthre version`；
- backend 可以被 `rclone config` / config string 识别；
- 不依赖修改 rclone 源码。

验收：

```text
rclone-chenthre version
rclone-chenthre listremotes
rclone-chenthre backend features <remote>:
```

均正常。

### P1 — Minimal Aliyun Backend

先实现最基础远端访问：

```text
lsf
mkdir
copy local → remote
copy remote → local
overwrite
rename / move（如果已接 optional feature）
```

此阶段 List 可以先复用已验证的 search fallback，但必须 fail closed。

目标不是优化，而是让 rclone 的普通操作真实跑通。

### P2 — Consistency Guard

加入：

- persistent catalog；
- known-file Search miss → GetFile verification；
- marker 完整分页；
- retry；
- dedup；
- parent correction；
- provider-internal path filtering；
- hidden trash。

此阶段要专门模拟 Search eventual consistency。

建议为 Search client 加测试注入层，使单测可以人为：

```text
第一次漏条目
第二次返回
```

并验证不会误判 known object 为删除。

### P3 — rclone Backend Contract / Integration Tests

使用 rclone 自身测试框架验证 Backend，而不是只写项目自己的 happy-path tests。

至少验证：

- Put；
- Get；
- List；
- Update；
- Remove；
- mkdir/rmdir；
- hash；
- object metadata；
- path/name；
- Unicode；
- spaces；
- nested dirs；
- duplicate names / auto rename 行为；
- zero-byte files；
- moderately large files；
- interrupted upload。

如果部分官方通用 test 与 Aliyun 明确不支持的 provider 特性冲突：

1. 先确认该能力是否 optional；
2. 记录 provider quirk；
3. 只有确实合理时才 skip；
4. 不得为了“全部绿”伪造 semantics。

### P4 — Bisync 双设备 PoC

在同一台 Ubuntu 模拟两台独立设备：

```text
testdata/device-a/
testdata/device-b/
```

两边使用不同的 bisync workdir，且测试过程必须有外层 single-flight lock。

初始首次同步：

1. 使用测试数据；
2. `--dry-run`；
3. 检查 plan；
4. 第一次真实建立 baseline 使用 `--resync`；
5. 后续正常轮次**禁止继续加 `--resync`**。

建议测试配置以：

```text
--compare size,checksum
--conflict-resolve none
--conflict-loser num
--check-access
--resilient
--recover
```

为基础，再根据所 pin rclone 版本确认最终参数。

`--max-delete` 应设置得比默认更保守，具体值由测试决定。

---

## 15. Bisync 验证矩阵

以下场景必须逐项自动或半自动执行并记录结果。

### 15.1 基础同步

```text
A 新建文件
→ B 得到同字节内容

B 新建文件
→ A 得到同字节内容
```

用 SHA-256 本地验证传输最终字节，而不仅看文件大小。

### 15.2 单端修改

```text
A 修改
→ B 更新

B 修改
→ A 更新
```

### 15.3 目录

```text
nested/a/b/c.md
```

必须能够往返同步，验证子目录 List workaround。

### 15.4 重命名 / 移动

测试：

```text
note.md → renamed.md
folder-a/note.md → folder-b/note.md
```

是否得到符合 bisync 语义的结果。

不强求第一版一定识别为 server-side rename；即使表现为 copy + logical remove，也必须保证最终内容正确。

### 15.5 删除

本地删除：

```text
A 删除 x.md
```

远端 Backend 应表现为 `Remove()` 成功：

```text
用户 namespace 中 x.md 不可见
provider-private trash 中仍物理存在
```

B 下一轮应删除本地副本。

### 15.6 冲突

在两次同步之间：

```text
A 修改 same.md → version A
B 修改 same.md → version B
```

要求：

- 不 silent overwrite；
- rclone conflict 行为符合配置；
- 双方内容至少都有一份可恢复；
- logs 能明确报告冲突。

### 15.7 Search 索引延迟

通过 mock 或真实快速写后同步制造：

```text
Search 暂时看不到 known file
```

要求：

> Backend 通过 catalog + GetFile 仍将 known object 正确暴露，绝不能向 rclone报告其被删除。

对于全新 remote object 暂时未进入 Search：

> 可以延迟到后续轮次发现。

### 15.8 网络失败

分别在：

- List；
- Search；
- Get；
- Upload；
- Download；

阶段模拟/制造失败。

要求：

```text
abort / retry
```

而不是产生错误删除或半文件。

### 15.9 进程中断

在 bisync 中途 kill。

下一次运行：

- 不破坏已有正确数据；
- `--recover` / resilient 行为符合预期；
- 必要时提示人工 resync；
- 不自动把半完成状态当最新 baseline。

### 15.10 API Key 失效

故意使用无效凭证。

必须：

```text
明显失败
0 destructive operations
```

### 15.11 空 listing 安全

人为让 provider listing 不可信或返回异常空。

Backend 应 return error / bisync safety abort。

不得因为：

```text
Search 空
```

把整个另一侧删除。

---

## 16. 安全 Invariants

实现过程中以下原则优先级高于“同步成功率”：

### 16.1 不确定时停止

```text
unknown state → abort
```

不：

```text
unknown state → assume deleted
```

### 16.2 不静默丢失冲突版本

双方都变更时不能默认 last-write-wins。

### 16.3 上传/下载后校验

在 Backend / rclone 能力允许时使用 checksum。

测试另外使用本地 SHA-256 验证最终字节一致。

### 16.4 同 workspace 单实例运行

Chenthre 最终必须保证：

```text
same workspace
→ at most one bisync run
```

即使 rclone 自身存在 lock，也保留 App-level single-flight mutex。

### 16.5 Provider-private 数据必须隐藏

所有：

```text
trash
catalog metadata（若云端存在）
internal check objects
```

不得混入用户正常 Vault listing。

---

## 17. Android 未来兼容约束

虽然 Android 不在本轮实施范围，但 Backend 设计不能把未来 Android 卡死。

rclone 官方提供 `librclone` 和 gomobile AAR 构建路径，因此：

```text
custom rclone build
+ aliyunenterprise backend
        ↓
gomobile / librclone
        ↓
Android
```

是未来候选方案。

因此当前必须避免：

- 依赖 Linux-only Go plugin 才能工作；
- 依赖 shell 脚本作为 Backend correctness 的必要组成；
- 依赖 external CLI process 才能完成 REST；
- 在 Backend 使用无法移植到 Android 的系统路径假设。

Android 未来会另外解决：

```text
SAF / content URI
background scheduling
secure key storage
SyncService facade
```

本阶段只为其保留可编译、可嵌入的结构。

需要注意：rclone `librclone` 官方目前仍将接口描述为实验性低层接口，因此 Android 集成需要包一层 Chenthre 自己的稳定 facade，不应让 UI 直接依赖 RPC JSON。

---

## 18. 测试数据要求

### 禁止

第一轮不使用真实主 Obsidian Vault。

### 建议创建

```text
test-vault/
├── note.md
├── 中文笔记.md
├── todo/
│   ├── today.md
│   └── nested/
│       └── idea.md
├── attachments/
│   ├── image.bin
│   └── empty.dat
└── .obsidian-test/
```

包含：

- 中文；
- 空格；
- Unicode；
- 零字节；
- 小文本；
- 中等大小二进制；
- 多层目录；
- 频繁覆盖。

---

## 19. 现有文件的处理

以下现有成果应保留并复用：

### `pds_list_children.py`

定位：

> Aliyun directory listing reference implementation / diagnostic tool。

将其算法迁移到 Go，但不要直接让 rclone shell out 调 Python。

### `test_pds_list_children.py`

保留作为：

> 服务端回归行为检测。

Go Backend 另外写自己的 test。

### `sync-delete.sh`

定位：

> Aliyun hidden-trash 行为参考和人工维护工具。

正式 Backend 的 `Remove()` 使用 Go REST Client 实现，不依赖 shell。

### `recheck.py`

保留：

> 服务端是否修复 ListFile / API 行为的 regression probe。

如果未来服务端修复子目录 List，Backend 可以内部走 native fast path，但 Search fallback 仍可保留一段时间作为兼容。

### docs / vendor / probe logs

保留为 provider research evidence，不应把实验性工具直接混入正式 sync implementation。

---

## 20. 错误处理与可观测性

Backend error 应至少区分：

```text
AuthenticationError
PermissionError
NotFound
RateLimited
TransientServerError
ConsistencyError
ProtocolError
LocalCatalogError
```

日志必须包含：

- operation；
- drive/path/file_id（可打印非敏感标识）；
- retry 次数；
- HTTP status；
- Aliyun error code；
- duration；
- fallback 是否触发。

不得包含：

- API Key；
- Authorization Header；
- upload signed URL 中可能存在的敏感 query。

Bisync 运行日志需要保存到测试输出目录，便于定位：

```text
为什么把某文件判定为 new / changed / deleted / conflict
```

---

## 21. 性能原则

当前是单用户个人 Vault，第一阶段优先：

```text
correctness > simplicity > performance
```

因此允许：

```text
Search + GetFile verification
```

产生比理想 API 更多请求。

暂时不做：

- 并行复杂缓存；
- 自研批量 RPC；
- block delta；
- ListR；
- aggressively cached stale listing。

等真实 Vault 规模出现性能瓶颈后，再用 profile 决定是否优化。

---

## 22. 失败判定 / 停止条件

如果出现以下任一情况，不要继续为了“让 rclone 能跑”而叠加 workaround，应停止并重新评估技术路线：

### A. 无法满足 List correctness

即使使用：

```text
Search + persistent catalog + GetFile
```

仍会稳定出现：

```text
known live object 被误报不存在
```

并可能导致 bisync 删除传播。

这是 rclone 路线的硬 blocker。

### B. rclone Backend contract 与 Aliyun 语义根本冲突

例如 bisync correctness 必须依赖某个我们无法模拟的 primitive。

### C. Hidden trash 无法可靠模拟 Remove

如果对象 Move 到 hidden trash 后仍会被普通 listing 暴露或路径行为不稳定。

### D. Android 无法实际构建自定义 librclone

这不阻塞 Ubuntu PoC，但在进入产品实现前必须重新评估。

如果 A/B/C 通过，才值得继续 Android。

---

## 23. 验收标准

### Backend 层

必须：

- [ ] out-of-tree 构建成功；
- [ ] pinned rclone version；
- [ ] 不需要修改官方 rclone 源码；
- [ ] API Key 配置安全；
- [ ] List root 正常；
- [ ] nested List 通过 fallback 正确；
- [ ] Search miss 不误判 known object delete；
- [ ] Put/Open/Update 正常；
- [ ] Hash(SHA1) 正常；
- [ ] Remove/Rmdir 通过 hidden trash 模拟；
- [ ] internal trash 不暴露；
- [ ] provider errors fail closed；
- [ ] rclone integration tests 达到可接受结果，所有 skip 有明确理由。

### Bisync 层

必须：

- [ ] 首次 dry-run + resync 正常；
- [ ] 后续普通 bisync 正常；
- [ ] A→B 新建；
- [ ] B→A 新建；
- [ ] A→B 修改；
- [ ] B→A 修改；
- [ ] nested dirs；
- [ ] rename/move；
- [ ] delete propagation；
- [ ] concurrent conflict 不丢数据；
- [ ] process interruption recovery；
- [ ] network error 不产生 destructive mis-sync；
- [ ] invalid API Key 不产生修改；
- [ ] Search eventual consistency 不造成 known-file false deletion；
- [ ] 本地最终文件 SHA-256 一致。

### 架构层

必须：

- [ ] Aliyun quirks 没有泄漏到 bisync 上层；
- [ ] Chenthre 后续可以用一个薄 `SyncService` 包装 rclone；
- [ ] provider 可替换；
- [ ] 不依赖 Linux-only plugin；
- [ ] Python probe 不成为生产运行依赖。

---

## 24. 最终交付物

执行完成后至少产生：

```text
rclone-aliyunenterprise/
    Go source
    tests
    pinned go.mod

rclone-chenthre
    reproducible build instructions

docs/
    aliyun-backend.md
    provider-quirks.md
    bisync-poc-results.md

tests / scripts
    backend integration test
    bisync scenario runner
```

`bisync-poc-results.md` 应明确记录：

- rclone 版本；
- Go 版本；
- Aliyun Backend commit；
- 测试矩阵；
- 每项 PASS/FAIL；
- 发现的 provider bug；
- 所有 workaround；
- 是否建议进入 Android 阶段。

---

## 25. 本阶段最终决策规则

PoC 完成后只做以下三种结论之一。

### ACCEPT

满足核心验收，特别是：

```text
List correctness
delete simulation
failure safety
conflict preservation
```

均可靠。

结论：

> 正式采用 rclone/bisync 作为 Chenthre 文件同步引擎。

下一阶段：

> Android 自定义 librclone/AAR + SAF Spike。

### ACCEPT WITH LIMITATIONS

主要功能可靠，但存在明确且可接受限制，例如：

- 新 remote object 发现有几秒延迟；
- hidden trash 需要人工周期清理；
- 部分 optional rclone capability 不支持。

要求限制必须：

- 不影响数据安全；
- 文档化；
- 对用户可解释。

### REJECT

如果 Aliyun listing 无法包装成可靠 rclone Fs contract，或者存在无法防止的 false deletion 风险：

> 停止继续适配 rclone。

此时再回到候选方案：

- 自研 append-only Sync Protocol；
- 更换远端服务；
- 使用其他成熟同步引擎。

不得因为已经投入代码而继续堆叠不可靠 workaround。

---

## 26. 参考资料

实施时优先阅读并以当前 pin 版本文档/源码为准：

- rclone Bisync  
  https://rclone.org/bisync/

- rclone Contribution / out-of-tree plugin/backend 说明  
  https://github.com/rclone/rclone/blob/master/CONTRIBUTING.md

- rclone Out-of-tree Backend Example  
  https://github.com/rclone/rclone_out_of_tree_example

- rclone librclone / gomobile  
  https://github.com/rclone/rclone/blob/master/librclone/README.md

- rclone Go API / `fs.Fs`  
  https://pkg.go.dev/github.com/rclone/rclone/fs

同时参考项目当前已经验证的：

```text
pds_list_children.py
test_pds_list_children.py
sync-delete.sh
recheck.py
docs/list问题-复现与复核说明.md
docs/实施结果说明-v1.md
```

这些文件代表当前 Aliyun Drive Enterprise 真实行为，不应被开发者版 PDS 文档中的理想行为覆盖。

---

## 27. 实施哲学

执行过程中遵循：

> **复用成熟同步算法，只自行实现项目独有的 Provider 差异。**

> **Adapter 可以复杂，Core 必须简单。**

> **一致性不确定时停止，而不是猜。**

> **宁可延迟同步，不可错误传播删除。**

> **测试目标不是“命令成功”，而是“不会静默丢数据”。**

> **先证明 Ubuntu PoC 的 correctness，再开始 Android。**
