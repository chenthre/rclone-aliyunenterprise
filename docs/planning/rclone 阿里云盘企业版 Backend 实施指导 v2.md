# rclone 阿里云盘企业版 Backend 实施指导 v2

> 适用项目：当前 `notes-sync` / `rclone-aliyunenterprise` 代码仓库  
> 当前阶段：Aliyun Enterprise Backend 稳定化与 rclone contract 收敛  
> 版本：v2  
>
> 本文替代《Chenthre Sync rclone接入实施指导 v1.md》中与“Chenthre App”绑定的表述。  
> 本项目的当前定位是：**为 rclone 提供一个可复用的阿里云盘企业版 Backend。**
>
> 该 Backend 理论上可被任何使用 rclone 的应用、脚本、CLI、librclone 集成或其他上层系统使用；不再把它设计成某一个特定 App 的私有同步模块。

---

## 1. 任务目标

### 1.1 当前项目定位

本阶段的目标不是开发一个具体 App，也不是实现一个新的同步算法。

目标是实现并稳定化：

> **一个独立、可复用、符合 rclone Backend contract 的 `aliyunenterprise` Backend，用于把阿里云盘企业版包装成 rclone 可访问的 Remote。**

整体结构应理解为：

```text
任意上层应用 / CLI / 自动化工具
              │
              ▼
            rclone
              │
      sync / copy / bisync / ...
              │
              ▼
        fs.Fs / Object contract
              │
              ▼
     aliyunenterprise Backend
              │
              ▼
      阿里云盘企业版 REST API
```

本项目只负责：

```text
Aliyun Drive Enterprise
        ↓
rclone Backend contract
```

而不负责：

- rclone 的同步算法；
- rclone bisync 的内部实现；
- 某个特定 App 的 UI；
- 某个特定 App 的业务逻辑；
- 自研 Sync Core；
- 自研冲突解决协议。

---

## 2. 关键架构原则

### 2.1 上层只面对 rclone

未来任何使用者都应该只看到：

```text
rclone remote
```

例如：

```text
aliyun:
```

或：

```text
:aliyunenterprise:
```

而不需要知道：

```text
file/list 子目录异常
file/search 最终一致
file_id
CheckRouterAccessFailed
DeleteFile 不可用
Aliyun API Key Bearer 细节
```

这些必须被封装在 Backend 内部。

### 2.2 只有 Backend contract 问题属于本项目必须修复的范围

问题分类原则：

| 问题 | 所属层 | 本项目是否负责 |
|---|---|---|
| Aliyun 子目录 `file/list` 为空 | Provider / Backend | 是 |
| Aliyun Search 最终一致 | Provider / Backend | 是，必须屏蔽危险语义 |
| Aliyun 无真 Delete API | Provider / Backend | 是，通过语义适配 |
| Aliyun 不能 SetModTime | Provider capability | 否，只需诚实声明 |
| rclone bisync conflict bug | rclone upstream | 否，归因、记录、上报 |
| rclone lock / workdir 行为 | rclone upstream | 否，按官方语义使用 |
| rclone bisync 策略限制 | rclone upstream | 否，接受/配置 |
| Android / librclone 集成问题 | 上层应用集成 | 否，不属于 Backend |
| 某个 App 的多设备协调策略 | 上层应用 | 否，不属于 Backend |

原则：

> **Backend 只负责忠实实现 rclone 需要的文件系统语义。**

> **rclone 自身的问题不在 Backend 中重新实现或修补。**

---

## 3. 必须删除的“Chenthre App”耦合

### 3.1 代码仓库去品牌化要求

本版本新增硬性要求：

> **删除代码仓库中所有将 Backend 绑定到 “Chenthre App” 的命名、字段、注释、文档和结构。**

需要检查并清理：

```text
Chenthre App
Chenthre SyncService
Chenthre application
Chenthre-specific app integration
App-specific workspace terminology
App-specific Android assumptions
```

如果某个名字只是历史路径、测试 artifact 或兼容层，必须判断是否仍有保留价值。

### 3.2 允许保留的项目名称

可以保留与 Backend 本身有关的中性名称，例如：

```text
rclone-aliyunenterprise
aliyunenterprise
```

当前：

```text
rclone-chenthre
```

属于历史 App/品牌耦合，建议改成：

```text
rclone-aliyunenterprise
```

或：

```text
rclone-aliyun
```

优先推荐：

```text
rclone-aliyunenterprise
```

因为它准确表达：

> rclone + Aliyun Drive Enterprise Backend

### 3.3 建议重命名

如果当前仓库存在：

```text
cmd/rclone-chenthre/
rclone-chenthre
```

建议迁移为：

```text
cmd/rclone-aliyunenterprise/
rclone-aliyunenterprise
```

如果仓库根模块本身已经是：

```text
github.com/chenthre/rclone-aliyunenterprise
```

其中 GitHub namespace `chenthre` 可以视为账户/组织 namespace，不等同于 App 品牌，不强制修改。

但代码内部不应再使用：

```text
Chenthre App
Chenthre Sync
Chenthre-specific
```

来描述 Backend 行为。

### 3.4 文档去耦

所有文档应从：

> “为 Chenthre App 提供同步”

改成：

> “为 rclone 提供 Aliyun Drive Enterprise Backend”

原先提到未来 Android App 的内容，应移动到：

```text
Non-goal / future consumer example
```

而不是成为当前设计约束。

---

## 4. 已确认的环境与 Provider 事实

### 4.1 服务类型

必须明确：

> 当前使用的是 **阿里云盘企业版（Aliyun Drive Enterprise）**。

没有：

```text
PDS Developer Edition
```

因此实现不得假设：

- RAM AccessKey；
- Developer Edition Domain；
- `*.api.aliyunpds.com`；
- Developer Edition OAuth；
- Developer Edition SDK-specific auth flow。

### 4.2 企业版 API Key

已实测：

```http
Authorization: Bearer <uk-...>
```

可以调用：

```text
https://<domain_id>.api.aliyunfile.com/v2/*
```

已验证：

- Drive 枚举；
- 根目录 List；
- Create File / Folder；
- Upload；
- multipart complete；
- Download；
- Get metadata；
- Search；
- Overwrite；
- Rename；
- Move；
- Copy；
- SHA-1 `content_hash`；
- 稳定 `file_id`。

配置必须外部注入。

禁止硬编码：

```text
domain_id
drive_id
API Key
```

### 4.3 Provider 已确认限制

#### 子目录 List 异常

```text
file/list(root)        → 正常
file/list(subfolder)   → items:[]
file/list_delta        → items:[]
```

#### Search 最终一致

刚上传/移动后：

- 可能短暂不可见；
- Move 后旧 parent 可能短暂 stale。

#### 无真正删除

```text
DeleteFile
TrashFile
```

对 API Key 返回：

```text
403 CheckRouterAccessFailed
```

#### ModTime

远端 `updated_at` 可读，但不能设置任意修改时间。

Backend 必须返回当前 pin 版本 rclone 对应的“不支持 SetModTime”语义，不得伪造。

---

## 5. 当前技术结论

当前 PoC 已确认：

> **采用 `rclone bisync + aliyunenterprise out-of-tree Backend` 的方向成立。**

但需要分层描述：

### Backend 架构决策

```text
ACCEPT
```

### Ubuntu PoC

```text
ACCEPT WITH LIMITATIONS
```

### Backend production readiness

```text
尚未最终完成
```

当前剩余工作不是继续开发同步算法，而是：

> **验证并稳定化 Backend 是否真正满足 rclone contract。**

---

## 6. Backend 目标语义

本项目目标不是：

> “让阿里云盘变成一个完整 POSIX 文件系统。”

而是：

> **Implement the minimum correct rclone Fs/Object semantics that Aliyun Enterprise can honestly support.**

Backend 至少应正确提供：

```text
Fs:
- NewFs
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
- ID（如当前 rclone contract/feature 支持）
```

以及所 pin rclone 版本要求的其他基础接口。

Optional features 只在 Provider 真实支持、且语义可靠时暴露。

---

## 7. Backend Out-of-Tree 结构

推荐继续使用：

```text
rclone-aliyunenterprise/
├── go.mod
├── backend/
│   └── aliyunenterprise/
│       ├── backend.go
│       ├── object.go
│       ├── client.go
│       ├── config.go
│       ├── listing.go
│       ├── catalog.go
│       ├── trash.go
│       ├── consistency.go
│       ├── errors.go
│       └── *_test.go
│
└── cmd/
    └── rclone-aliyunenterprise/
```

不修改 rclone 官方源码。

依赖：

```text
github.com/rclone/rclone vX.Y.Z
```

必须 pin。

不要跟踪 `master`。

---

## 8. List() correctness —— Backend 最关键职责

### 8.1 rclone 的假设

当 rclone 调：

```go
Fs.List(ctx, dir)
```

Backend 必须提供：

> 当前目录直属对象的可信集合。

如果把“暂时 Search 不到”解释成：

```text
不存在
```

可能导致 rclone/bisync 传播删除。

因此：

> **List correctness 是 Aliyun Backend 的首要安全约束。**

### 8.2 当前 fallback

```text
ListChildren(parent)
    │
    ▼
file/list
    │
    ├─ root → 使用
    ├─ non-empty → 使用
    │
    └─ 子目录返回空
            ↓
file/search(
  parent_file_id=<parent>,
  recursive=false
)
            ↓
marker 全分页
            ↓
file/get 逐项复核
            ↓
按真实 parent/status 过滤
```

### 8.3 Search miss invariant

必须保留：

> **Search absence != confirmed deletion**

对于 catalog 中已知对象：

```text
Search miss
    ↓
GetFile(file_id)
    │
    ├─ exists + same parent
    │     → 补回 List
    │
    ├─ exists + moved
    │     → 更新 catalog
    │
    └─ authoritative not-found
          → 才允许认为消失
```

### 8.4 Fail Closed

如果：

- Search 连续失败；
- pagination 不完整；
- GetFile 大规模失败；
- auth 失败；
- 429/5xx 超过 retry；
- catalog 不能可靠读取；

Backend 必须：

```text
return error
```

不得：

```text
return partial/empty list
```

---

## 9. Provider Safety Catalog

### 9.1 定位

Catalog 不再被定义为普通性能 cache。

它属于：

> **Aliyun Backend 为满足 List correctness 所需的 provider-specific safety state。**

因为：

```text
Search eventual consistency
```

意味着 catalog 参与避免 false deletion。

### 9.2 最小字段

建议：

```text
schema_version
provider_type
domain_id
drive_id
remote_root

objects:
  file_id
  parent_file_id
  name
  remote_path
  size
  sha1
  modified_at
  last_seen_at
```

### 9.3 持久化要求

必须：

- atomic write；
- schema version；
- remote/workspace identity；
- corruption detection；
- 错误时 fail closed。

简单实现即可：

```text
catalog.json.tmp
    ↓
write
    ↓
fsync
    ↓
atomic rename → catalog.json
```

建议同时保留：

```text
catalog.json.bak
```

### 9.4 错误场景

必须测试：

```text
catalog corrupt
→ error / safe recovery

catalog belongs to wrong drive
→ reject

partial write
→ recover old valid version or error

catalog missing
→ 明确重建流程，不静默假设空 remote
```

注意：

Catalog 只属于：

```text
aliyunenterprise Backend
```

不要把它暴露成 rclone 上层状态。

---

## 10. 删除语义 —— Backend Adapter 行为

### 10.1 Remove()

rclone 调：

```text
Object.Remove()
```

上层语义是：

```text
对象从用户 namespace 消失
```

Aliyun 没有 Delete，因此 Backend 内部实现：

```text
MoveFile
    ↓
provider-private trash
```

例如：

```text
_aliyunenterprise_rclone_internal/
    trash/
```

建议移除：

```text
_chenthre_*
```

等旧 App/品牌相关内部目录名称。

### 10.2 Internal namespace

推荐使用 Provider 中性内部目录：

```text
_aliyunenterprise_rclone_internal/
```

或：

```text
.rclone-aliyunenterprise/
```

需要考虑 Aliyun 对点目录/特殊名字行为，选择经验证稳定的名称。

推荐结构：

```text
_aliyunenterprise_rclone_internal/
├── trash/
└── optional-metadata/
```

### 10.3 Trash 唯一命名

避免同名：

```text
<timestamp>-<file_id>-<escaped_name>
```

或：

```text
<file_id>/<original_name>
```

### 10.4 上层隐藏

普通：

```text
List
NewObject
lsf
lsd
bisync
```

不得看到内部 trash。

### 10.5 GC

Physical delete / trash GC 当前仍为非目标。

---

## 11. Provider Capabilities —— 诚实声明

### 11.1 ModTime

Provider 不能 SetModTime：

```text
不伪造
不建立自定义远端 mtime DB
```

正确返回 unsupported。

上层 rclone 自己选择：

```text
checksum
size
```

等策略。

### 11.2 ListR

当前不要实现：

```text
ListR
```

因为 Search 最终一致，不足以作为可靠 recursive listing contract。

保持：

```text
Features().ListR = nil
```

让 rclone 自己递归调用 `List()`。

### 11.3 Move / Copy

Aliyun 已支持 server-side：

```text
Move
Copy
```

可以作为 optional feature 暴露。

前提：

> 行为必须通过 contract/integration test。

### 11.4 Purge

需要清理当前文档/代码语义。

必须明确：

```text
Features().Purge
```

到底：

- 已正式暴露并符合 rclone contract；
- 还是只有内部 helper；
- 或未支持。

不允许同时出现：

```text
“Purge 已实现”
```

和：

```text
“Purge 不支持”
```

的矛盾文档。

如果 Aliyun 无真删除，Purge 如果实现，也只能：

> 把整个目录树移动到 provider-private trash。

只有当 rclone contract 允许这种可观察语义时才暴露。

---

## 12. Hash

Aliyun 提供：

```text
SHA-1 content_hash
```

Backend 应：

- 声明 SHA-1；
- 标准化大小写；
- 返回 rclone 期望格式；
- 真实内容与 hash 行为通过测试。

不要自行把 SHA-1 改成 SHA-256 作为 Backend API。

rclone 本身支持哪些 hash，由 rclone contract 决定。

测试层可以额外使用 SHA-256 验证字节完整性，但那不是 Provider contract。

---

## 13. rclone 自身问题的处理原则

### 13.1 不修 rclone

如果问题可以在：

```text
official backend / local ↔ local
```

中复现，则分类：

```text
UPSTREAM RCLONE BUG / LIMITATION
```

本项目只做：

- 最小复现；
- 记录；
- 上报；
- pin 版本；
- 必要时文档说明可用配置规避。

不在 Backend 中：

- 重写冲突算法；
- 重写 bisync baseline；
- 重写 lock；
- 自研 recovery layer；
- 加入 provider-specific hack 改变 rclone 上层行为。

### 13.2 `conflict2 missing info`

当前已观察：

```text
bisync.renames: internal error:
missing info for "...conflict2"
```

下一步只做**归因**。

#### 实验 A

使用：

```text
local filesystem ↔ local filesystem
```

或 rclone 官方 Backend 做最小 reproducer。

如果复现：

```text
UPSTREAM
```

结束本项目修复尝试。

#### 实验 B

如果只在：

```text
aliyunenterprise
```

复现：

检查：

- List semantics；
- Object ID；
- Move；
- Copy；
- rename；
- Hash；
- metadata；
- ModTime；
- path normalization。

只有确认 Backend contract violation 后才修。

---

## 14. 不再属于当前 Backend 范围的问题

以下内容从当前 v2 实施计划中移除：

### Remote distributed lease

不实现。

原因：

> 多设备同时运行 bisync 属于上层 rclone 使用模型 / 应用协调问题，不属于 Aliyun Backend contract。

### 自定义冲突解决

不实现。

使用 rclone bisync 自身策略。

### rclone lock 修复

不实现。

上游问题。

### 自研 SyncService

不属于本 Backend 仓库。

未来任何 App 可以自己包：

```text
rclone CLI
librclone
RC API
```

Backend 不对具体 App 提供 facade。

### Android 专用逻辑

不实现。

未来若某 Android App 使用此 Backend，应通过：

```text
rclone / librclone
```

接入。

Backend 本身应保持通用。

---

## 15. P5 —— Aliyun Backend Stabilization

当前下一阶段正式调整为：

> **只做 Backend 自己应该负责的稳定化。**

### P5.1 官方 rclone Backend contract / fstest

最高优先级。

使用所 pin rclone 版本的官方测试框架。

测试结果分类：

```text
PASS
UNSUPPORTED OPTIONAL FEATURE
BACKEND BUG
UPSTREAM BUG / TEST ASSUMPTION
```

处理原则：

```text
BACKEND BUG
→ 修

UNSUPPORTED OPTIONAL
→ 正确声明 capability，不修

UPSTREAM
→ 记录 / 上报，不修 Backend
```

### P5.2 Catalog durability

完成：

- atomic persistence；
- schema version；
- remote identity；
- corruption detection；
- backup/recovery；
- fail closed。

### P5.3 Hidden trash contract

验证：

```text
Remove
Rmdir
Purge（如暴露）
```

上层语义正确。

必须覆盖：

```text
Remove 后 NewObject(path) → not found
List 不看到 trash
重复同名删除无冲突
trash 内对象不会重新进入普通 remote namespace
```

### P5.4 conflict2 attribution

只做：

```text
upstream vs backend
```

归因。

### P5.5 文档与命名去 Chenthre App 化

必须：

- rename binary；
- rename internal trash namespace；
- 删除文档中的 App-specific architecture；
- 删除代码注释中的 Chenthre App；
- 删除 App-specific TODO；
- 更新 README；
- 更新 module/repo usage 示例；
- 确保 README 表述为：

> rclone backend for Aliyun Drive Enterprise

---

## 16. 官方 Contract 测试范围

至少覆盖：

```text
Put
Open
Update
List
NewObject
Mkdir
Rmdir
Remove
Hash
Size
ModTime
Unicode
spaces
nested directories
zero-byte files
duplicate names
special characters
large-ish file
interrupted upload
Move
Copy
```

如果官方测试包含 optional feature：

必须先确认当前 Backend 是否声明支持。

不得：

> 为了让测试全部 PASS 而伪造不真实 Provider 能力。

---

## 17. Provider 错误映射

Backend 至少区分：

```text
AuthenticationError
PermissionError
ObjectNotFound
DirNotFound
AlreadyExists
RateLimited
TransientServerError
ConsistencyError
ProtocolError
CatalogError
```

并映射到：

```text
rclone expected errors
```

具体类型以 pin 版本源码为准。

所有“不确定但可能导致 destructive interpretation”的错误必须：

```text
return error
```

而不是空列表/NotFound。

---

## 18. 安全原则

### 18.1 Fail Closed

```text
unknown
→ error
```

不是：

```text
unknown
→ missing
```

### 18.2 Search absence != deletion

必须保持。

### 18.3 Internal namespace 不泄漏

Provider-private：

```text
trash
metadata
catalog-related remote artifact（如果未来有）
```

不得出现在正常 rclone namespace。

### 18.4 Secret 不泄漏

不得：

- commit API Key；
- 打印 Bearer；
- 打印 signed URL；
- 写入测试 fixture；
- 写入 README。

---

## 19. 非目标

当前 v2 明确不做：

- 任何具体 App；
- “Chenthre App”；
- Android UI；
- Android SAF；
- iOS；
- App SyncService；
- WorkManager；
- 远端分布式锁；
- 自研 Sync Core；
- 自研 bisync；
- 自研 conflict engine；
- CRDT；
- P2P；
- 真删除；
- GC；
- E2EE；
- 块级 delta；
- rclone fork；
- 修复 rclone upstream bug。

---

## 20. 代码仓库清理检查表

执行 Agent 必须搜索整个仓库：

```bash
grep -Rni "Chenthre App" .
grep -Rni "Chenthre Sync" .
grep -Rni "rclone-chenthre" .
grep -Rni "_chenthre" .
grep -Rni "SyncService" .
```

根据语义逐项处理。

### 必须删除/改名

App-specific：

```text
Chenthre App
Chenthre SyncService
rclone-chenthre binary
_chenthre_rclone_internal
_chenthre_rclone_trash
```

推荐替换：

```text
rclone-aliyunenterprise
_aliyunenterprise_rclone_internal
_aliyunenterprise_rclone_trash
```

### 不一定需要修改

如果 `chenthre` 只是：

```text
GitHub account namespace
author identity
historical commit metadata
```

不是 App coupling，可以保留。

不要做无意义的 namespace 大迁移。

---

## 21. 文档结构调整

推荐最终仓库：

```text
rclone-aliyunenterprise/
├── README.md
├── go.mod
├── backend/
│   └── aliyunenterprise/
├── cmd/
│   └── rclone-aliyunenterprise/
├── docs/
│   ├── provider-quirks.md
│   ├── implementation-notes.md
│   ├── fstest-results.md
│   └── bisync-poc-results.md
└── tools/
    └── provider probes
```

README 重点：

1. 这是一个 rclone Backend；
2. 支持 Aliyun Drive Enterprise；
3. 需要企业版 API Key；
4. 不需要 PDS Developer Edition；
5. 当前 Provider limitations；
6. 如何构建；
7. 如何配置；
8. 已支持 Features；
9. 已知 upstream limitations；
10. Secret 安全说明。

---

## 22. 测试与验收

### Backend contract

必须：

- [ ] pinned rclone version；
- [ ] out-of-tree build；
- [ ] 无官方源码修改；
- [ ] Backend 注册；
- [ ] root List；
- [ ] nested List；
- [ ] Search fallback；
- [ ] known Search miss rescue；
- [ ] catalog corruption fail closed；
- [ ] wrong drive catalog rejected；
- [ ] Put/Open/Update；
- [ ] Hash SHA-1；
- [ ] Remove；
- [ ] Rmdir；
- [ ] internal trash hidden；
- [ ] Move/Copy（如声明）；
- [ ] unsupported features 正确 nil / error；
- [ ] fstest/integration 达到预期。

### Secret

- [ ] API Key 不在 Git；
- [ ] log 脱敏；
- [ ] signed URL 不打印。

### Naming / Scope

- [ ] 无 `Chenthre App`；
- [ ] 无 App-specific SyncService；
- [ ] binary 改为中性名称；
- [ ] internal namespace 去 App 品牌；
- [ ] README 定位为 generic rclone Backend；
- [ ] Android 仅作为潜在消费者示例，非当前 scope。

### conflict bug

- [ ] 完成 local/reference reproducer；
- [ ] 分类为 upstream 或 backend；
- [ ] 若 upstream：记录 issue/reproducer，不在 Backend 修；
- [ ] 若 backend：找到 contract violation 并修复。

---

## 23. 当前 rclone/bisync PoC 结论的保留

现有 PoC 已确认以下行为，不需要重新从零证明：

```text
A→B 新建
B→A 修改
nested dirs
rename
move
logical delete
hash integrity
Search eventual consistency fallback
API Key invalid fail closed
kill/recovery 基本行为
冲突至少不静默丢失版本
```

这些继续作为 regression suite。

但它们的定位是：

> **Backend + rclone integration tests**

不是本 Backend 自己实现的同步能力。

---

## 24. 下一阶段之后

如果 P5 完成，Backend 达到稳定状态，则：

> Backend 项目进入维护模式。

之后任何上层系统可以选择：

```text
rclone CLI
RC API
librclone
gomobile
system service
cron
desktop app
mobile app
```

来使用该 Backend。

是否开发 Android App、iOS App、AI 助手或其他产品：

> 属于独立上层项目，不进入本 Backend 仓库的职责范围。

---

## 25. 最终交付物

P5 完成后至少应有：

```text
rclone-aliyunenterprise/
├── README.md
├── pinned go.mod
├── backend/aliyunenterprise/
├── cmd/rclone-aliyunenterprise/
├── docs/
│   ├── provider-quirks.md
│   ├── fstest-results.md
│   ├── bisync-poc-results.md
│   └── upstream-issues.md
└── tests/
```

并且提供：

```text
reproducible build
documented config
contract test command
integration test command
known limitations
```

---

## 26. 最终验收结论规则

### ACCEPT

满足：

- rclone mandatory contract；
- List correctness；
- catalog durability；
- hidden trash semantics；
- error fail closed；
- naming 去 App 耦合；
- upstream/backend 问题已归因。

则：

> `aliyunenterprise` Backend 进入稳定维护阶段。

### ACCEPT WITH LIMITATIONS

允许：

- Search 新对象发现秒级延迟；
- 无真 Delete；
- 无 SetModTime；
- 无 ListR；
- 无 ChangeNotify；
- upstream bisync 已知 bug。

前提：

> 不造成 Backend 对 rclone contract 的错误承诺。

### REJECT

仅当：

```text
Aliyun Provider 的真实语义无法被安全包装成 rclone mandatory Fs/Object contract
```

才否定该 Backend 路线。

rclone 自身 bug：

> **不是 Reject Backend 的理由，除非其限制使所有目标使用场景不可接受。**

---

## 27. 实施哲学

> **这是一个 rclone Backend，不是一个 App。**

> **上层面对 rclone，Backend 面对 Aliyun。**

> **只有 Backend contract violation 才是本项目必须修的问题。**

> **Provider 能力弱时，诚实声明弱能力，不伪造能力。**

> **rclone 的 bug 属于 upstream；归因、记录、上报，不在 Backend 重新实现 rclone。**

> **Search 不确定时失败关闭，绝不把不确定解释成删除。**

> **保持 Backend 通用，使任何 rclone 用户都能使用。**
