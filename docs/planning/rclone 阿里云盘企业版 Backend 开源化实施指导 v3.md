# rclone 阿里云盘企业版 Backend 开源化实施指导 v3

> 当前阶段：从内部可用 Backend 走向**标准、可开源、可复用、适合他人使用**的 rclone Backend  
> 目标仓库名：`rclone-aliyunenterprise`  
> 目标用户：希望使用**阿里云盘企业版**作为备份、同步、归档或 rclone Remote 的个人用户与团队  
> 上游依赖：rclone `v1.75.1`（当前 pinned；升级需重新跑完整测试）
>
> 本阶段的最终目标是：**把当前可工作的实现整理成一个符合优秀开源软件实践、别人可以独立安装、配置、验证、使用和贡献的公开仓库。**

---

## 1. 项目使命

本项目解决的问题是：

> 阿里云盘企业版具备可用的 API Key 与文件 REST API，但没有官方 rclone Backend；同时其 API 在目录枚举、删除、修改时间等方面存在与标准文件系统/rclone 语义不完全一致的限制。

本仓库提供：

```text
rclone
   │
   ▼
aliyunenterprise Backend
   │
   ├─ 企业版 API Key Bearer 认证
   ├─ Aliyun REST Client
   ├─ 目录一致性补偿
   ├─ Search fallback + GetFile verification
   ├─ known-object safety catalog
   ├─ hidden-trash 删除语义适配
   └─ SHA-1 / Move / Copy capability 映射
   │
   ▼
Aliyun Drive Enterprise
```

从使用者视角，应表现为一个普通 rclone remote：

```bash
rclone lsf aliyun:
rclone copy ~/Documents aliyun:backup
rclone sync ~/Vault aliyun:vault
rclone bisync ~/Vault aliyun:vault
```

用户不需要理解底层 Provider quirks。

---

## 2. 本阶段任务目标

本阶段工作重点从“验证技术可行”转向：

1. 完成 rclone Backend contract 稳定化；
2. 接入并跑通官方 `fstest` / integration test；
3. 建立标准开源仓库结构；
4. 完善公共使用文档；
5. 建立 CI；
6. 建立版本与发布流程；
7. 明确安全模型；
8. 明确 Provider limitations；
9. 提供别人可复现的安装、配置和验证流程；
10. 准备首个公开版本 `v0.1.0`。

完成后，一个没有参与此前开发的用户，应仅凭 README 和文档完成：

```text
Clone / Download
      ↓
Build
      ↓
Configure Aliyun Enterprise API Key
      ↓
Create rclone remote
      ↓
Run lsf / copy / sync / bisync
      ↓
理解限制与安全注意事项
```

---

## 3. 当前状态

### 3.1 工程

已完成：

- out-of-tree rclone Backend；
- 不修改 rclone 官方源码；
- pinned rclone `v1.75.1`；
- Go module 可独立构建；
- binary 已去特定 App 品牌；
- README 已改成 generic Backend 定位；
- secret / binaries / runtime data 已通过 `.gitignore` 隔离。

### 3.2 Provider 能力

已验证：

```text
List
NewObject
Put
Open
Update
Mkdir
Rmdir
Remove（逻辑删除）
Hash(SHA-1)
Size
ModTime(read-only)
Move
Copy
```

### 3.3 Provider workaround

已实现：

```text
Native list
   ↓
Search fallback
   ↓
GetFile authoritative verification
   ↓
persistent catalog
```

并遵守：

> `Search absence != confirmed deletion`

### 3.4 Catalog

已完成：

- schema v2；
- remote identity binding；
- atomic write；
- fsync；
- `.bak`；
- corruption fail-closed；
- wrong-drive reject。

### 3.5 Hidden trash

已完成：

- `Remove` 后普通 namespace 不可见；
- 同名文件可重新创建；
- 重复删除幂等；
- trash 不混入普通 listing。

### 3.6 已修复的重要 Backend bug

已确认并修复：

- `Move/Copy` 返回稀疏 metadata 导致 bisync conflict2 内部错误；
- root `file/list` 同样存在最终一致问题；
- 并发同名目录歧义；
- `NewCatalog` 未 load 导致进程重启后 catalog 失效。

---

## 4. 本项目责任边界

### 4.1 本仓库负责

```text
Aliyun Enterprise API
        ↓
correct rclone Fs/Object semantics
```

包括：

- Provider API 适配；
- error mapping；
- listing correctness；
- capability declaration；
- provider-specific safety state；
- hidden trash；
- secret handling；
- integration tests；
- public documentation。

### 4.2 本仓库不负责

- rclone sync/bisync 算法；
- rclone upstream conflict policy；
- rclone upstream lock 行为；
- GUI / Android / iOS App；
- 自研同步协议；
- CRDT / P2P；
- 修复 rclone upstream bug。

如果问题可以在官方 Backend 或 local↔local 中复现，则归类：

```text
UPSTREAM RCLONE ISSUE
```

只做最小复现、记录、上报和必要的使用说明。

---

## 5. 项目命名与仓库整理

### 5.1 最终仓库名

推荐：

```text
rclone-aliyunenterprise
```

如果 Git 仓库仍名为：

```text
notes-sync
```

本阶段完成后应 rename。

### 5.2 统一命名

```text
Repository:
rclone-aliyunenterprise

Backend:
aliyunenterprise

Binary:
rclone-aliyunenterprise
```

Go module 采用实际仓库 owner：

```text
<owner>/rclone-aliyunenterprise
```

不得再出现具体 App 品牌耦合词。

---

## 6. 推荐仓库结构

```text
rclone-aliyunenterprise/
├── .github/
│   ├── workflows/
│   │   ├── ci.yml
│   │   ├── integration.yml
│   │   └── release.yml
│   ├── ISSUE_TEMPLATE/
│   │   ├── bug_report.yml
│   │   ├── provider_issue.yml
│   │   └── feature_request.yml
│   └── pull_request_template.md
│
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
│       ├── *_test.go
│       └── integration_test.go
│
├── cmd/
│   └── rclone-aliyunenterprise/
├── docs/
│   ├── configuration.md
│   ├── provider-quirks.md
│   ├── safety.md
│   ├── fstest-results.md
│   ├── bisync-poc-results.md
│   ├── troubleshooting.md
│   └── upstream-issues.md
├── tools/
├── .gitignore
├── .golangci.yml
├── CHANGELOG.md
├── CONTRIBUTING.md
├── LICENSE
├── README.md
├── SECURITY.md
├── go.mod
└── go.sum
```

历史研究材料可归档到：

```text
docs/project-history/
```

---

## 7. P6.1：官方 rclone Backend Contract 测试

这是当前**最高优先级任务**。

### 7.1 目标

接入 pinned rclone 版本提供的 Backend test / `fstest` / integration framework。

目标不是“所有测试强行变绿”，而是：

> 确认 Backend 对 rclone 声明的每一个 contract 都真实、稳定、可重复。

### 7.2 失败分类

每个测试结果必须归类：

```text
PASS

BACKEND BUG
→ 必须修

UNSUPPORTED OPTIONAL FEATURE
→ 不修，正确关闭 capability

PROVIDER LIMITATION
→ 判断是否可安全映射；不可则声明 unsupported

UPSTREAM RCLONE BUG / TEST ASSUMPTION
→ 记录，不在 Backend 内重写 rclone
```

### 7.3 必须覆盖

至少：

```text
NewFs
List
NewObject
Put
Open
Update
Mkdir
Rmdir
Remove
Hash
Size
ModTime
Unicode
spaces
zero-byte files
nested directories
special characters
duplicate names
range reads（如 contract 要求）
large-ish file
Move
Copy
retry/error semantics
not-found semantics
directory not-found semantics
```

### 7.4 输出

更新：

```text
docs/fstest-results.md
```

任何 FAIL 都必须有明确分类，不允许“未知原因失败”。

---

## 8. CI

### 8.1 PR CI

无需真实密钥：

```bash
go test ./...
go vet ./...
```

可行时增加：

```bash
go test -race ./...
```

并检查：

```text
gofmt
go mod tidy
staticcheck / golangci-lint
```

### 8.2 Integration CI

真实 Aliyun test：

- 单独 workflow；
- 使用 GitHub Actions Secrets；
- fork PR 默认不运行；
- 支持 `workflow_dispatch`；
- 可以对 main / release branch 定时运行；
- 使用隔离 test prefix；
- 不打印 API Key / signed URL。

Secrets：

```text
ALIYUN_ENTERPRISE_API_KEY
ALIYUN_ENTERPRISE_DOMAIN_ID
ALIYUN_ENTERPRISE_DRIVE_ID
```

### 8.3 Test cleanup

由于没有真 Delete，integration test 不应依赖彻底清理。

测试数据应隔离，并允许最后 move 到 internal trash 或输出人工清理清单。

---

## 9. README 标准

README 至少包括：

1. 一句话介绍；
2. Status（当前建议 Beta）；
3. Requirements；
4. Installation；
5. Configuration；
6. Basic Usage；
7. Sync / Bisync；
8. Known Limitations；
9. Security；
10. Unofficial project disclaimer。

推荐开头：

> `rclone-aliyunenterprise` is a community-maintained out-of-tree rclone backend for Aliyun Drive Enterprise. It enables Aliyun Drive Enterprise storage to be used with standard rclone commands such as `copy`, `sync`, and `bisync`.

并明确：

> This project targets Aliyun Drive Enterprise API Keys. It is not a PDS Developer Edition backend.

---

## 10. Bisync 文档

README / safety docs 应说明：

- bisync 是 rclone 本身功能；
- 本 Backend 已做实际 PoC；
- 首次运行遵循 rclone 官方 `--resync` 流程；
- 后续普通运行不要持续使用 `--resync`；
- 推荐 `--compare size,checksum`；
- 重要数据先 `--dry-run`；
- 先使用备份/测试数据；
- 不承诺“绝不会丢数据”。

---

## 11. 已知限制必须公开

至少包括：

- List/Search eventual consistency；
- 新对象可能数秒延迟发现；
- API Key 不支持真 Delete；
- Remove 实际 move 到 hidden trash；
- trash 需要人工周期清理；
- SetModTime unsupported；
- ListR unsupported；
- ChangeNotify unsupported；
- provider revision history 不可靠；
- provider-specific catalog 是 listing correctness safety state。

---

## 12. 安全文档

建立：

```text
SECURITY.md
docs/safety.md
```

要求用户：

- API Key 不 commit；
- 不贴到 issue；
- 不贴完整 debug log；
- 尽量最小权限；
- 仅授权目标 Drive；
- 设置有效期；
- 泄露后立即 revoke。

日志必须自动 redaction：

```text
Authorization: Bearer ***
```

Signed upload/download URL 默认不得输出。

还应声明：

> 本项目是非官方社区项目，与 Alibaba Cloud / Aliyun 官方无隶属关系，除非未来获得明确授权。

---

## 13. License

公开前必须选择 LICENSE。

优先候选：

```text
MIT
Apache-2.0
```

由于 rclone 本身使用 MIT License，MIT 是简单候选。

但发布前必须：

1. 检查是否复制了第三方源码；
2. 检查 vendor / skill / probe 内容；
3. 不公开未经许可的官方 Skill archive；
4. 保留必要版权声明。

---

## 14. Third-party / Vendor 清理

公开前检查：

```text
vendor/
official skill zip
downloaded docs
copied source
probe fixtures
```

原则：

> 不确定是否允许再分发的第三方文件，不进入公开仓库。

可以保留：

- 自己写的 probe；
- 脱敏最小 fixture；
- 自己写的测试。

---

## 15. Config UX

公共 Backend 不应要求用户改源码或手工改 JSON。

目标是：

```text
rclone config
```

可以完成配置。

至少需要：

```text
domain_id
drive_id
api_key
```

`api_key` 必须为 sensitive/password option，不在 CLI 输出明文。

未来可增加 Drive 枚举选择，但不是 v0.1 blocker。

---

## 16. Error UX

公共用户应看到有意义错误，例如：

```text
authentication failed: API key invalid or expired

drive not found or not accessible

ambiguous remote path: multiple folders with the same name

listing consistency check failed; refusing to return incomplete directory

provider does not support setting modification times

physical deletion is not supported by Aliyun Enterprise API Keys; object moved to internal trash
```

同时可附 Aliyun error code。

---

## 17. Provider Safety Catalog 公共化

Catalog 属于 correctness state，不是普通性能缓存。

### 17.1 路径

不得继续硬编码个人品牌目录。

使用：

```text
~/.cache/rclone-aliyunenterprise/
```

或更推荐 rclone/OS 标准 cache API。

### 17.2 Remote 隔离

至少按：

```text
domain_id
drive_id
remote root
```

隔离。

### 17.3 Lifecycle

文档明确：

- catalog 不应随便删除；
- corruption 会 fail closed；
- missing catalog 需要明确 rebuild；
- 不允许静默将“无 catalog”解释成“空 remote”。

---

## 18. Hidden Trash 公共行为

用户调用：

```text
rclone delete
rclone sync
rclone bisync
```

涉及删除时，Backend 实际：

```text
Move → internal trash
```

因此必须公开说明：

> “Deleted” means hidden from the rclone-visible namespace, not physically removed from Aliyun storage.

需要说明：

- internal trash 位置；
- 如何人工清理；
- 会继续占用云盘空间；
- v0.1 不提供自动 GC。

---

## 19. Upstream rclone 兼容策略

当前 pin：

```text
rclone v1.75.1
```

每次升级：

```text
update go.mod
     ↓
unit test
     ↓
fstest
     ↓
real integration
     ↓
bisync regression
```

全部通过后才合并。

依赖更新 bot 可以后续启用，但 rclone 更新不得自动 merge。

---

## 20. Versioning

使用 Semantic Versioning。

第一公开版本建议：

```text
v0.1.0
```

含义：

> Beta but usable.

建议：

```text
v0.1.x  → bugfix / contract correction
v0.2.0  → new optional capability / UX
v1.0.0  → config/layout/API considered stable
```

---

## 21. CHANGELOG

建立：

```text
CHANGELOG.md
```

使用：

```text
Added
Changed
Fixed
Security
Known Issues
```

首版至少记录：

- API Key auth；
- List/Search/Get consistency；
- safety catalog；
- hidden trash；
- SHA-1；
- Move/Copy；
- bisync tested；
- known Provider limitations。

---

## 22. CONTRIBUTING

建立：

```text
CONTRIBUTING.md
```

Bug report 要求提供：

- rclone version；
- Backend version；
- Go version（如源码构建）；
- command；
- sanitized debug log；
- minimal reproduction。

禁止贴：

- API Key；
- 完整 signed URL。

Issue/PR 应主动区分：

```text
Backend bug
vs
rclone upstream bug
```

Provider-specific workaround 必须有 regression test。

---

## 23. Issue Templates

至少建立：

### Bug Report

Backend bug。

### Provider/API Issue

Aliyun API 行为变化、Search/List 语义变化、权限问题等。

### Feature Request

可选 capability / UX。

明确不接受在 Backend 中重新实现：

```text
bisync
sync algorithm
conflict engine
app orchestration
```

---

## 24. 文档

### `docs/configuration.md`

- API Key；
- domain_id；
- drive_id；
- rclone config。

### `docs/provider-quirks.md`

- eventual consistency；
- List behavior；
- no true delete；
- no SetModTime；
- no ListR；
- revision behavior；
- endpoint/auth facts。

### `docs/safety.md`

- catalog；
- hidden trash；
- bisync；
- backup；
- dry-run；
- credentials。

### `docs/troubleshooting.md`

至少：

```text
401 AccessTokenInvalid
403 CheckRouterAccessFailed
ambiguous folder
catalog corruption
search consistency failure
API Key expired
bisync resync needed
```

### `docs/upstream-issues.md`

只记录已确认 upstream 的问题。

---

## 25. Test 分层

### Layer 1 — Unit

离线：

```text
catalog
error mapping
path handling
trash naming
listing merge
search miss rescue
```

### Layer 2 — Mock Provider

模拟：

```text
Search stale
List empty
Get authoritative
429
5xx
401
duplicate directories
```

### Layer 3 — Real Integration

真实 Aliyun Enterprise。

### Layer 4 — rclone Scenario Regression

```text
copy
sync
bisync
rename
delete
conflict
nested
```

---

## 26. 永久 Regression Cases

必须永久保留：

### root listing eventual consistency

防止以后重新假设：

```text
root native List is authoritative
```

### Search miss known object

```text
Search miss
+ catalog hit
+ Get exists
→ object remains visible
```

### Move/Copy sparse response

```text
Move/Copy sparse metadata
→ GetFile refresh
```

### Duplicate folders

```text
same parent
same name
different file_id
→ fail closed
```

### Catalog load

新进程必须 load 持久状态。

---

## 27. 当前明确不做

为防 scope creep，v0.1 前不要投入：

- Android；
- iOS；
- GUI；
- FUSE；
- AI；
- 自研 Sync Core；
- distributed lock；
- CRDT；
- provider true delete workaround；
- automatic trash GC；
- fork rclone；
- patch bisync；
- block-level delta。

---

## 28. Secret Audit

公开前执行 Git 历史扫描。

重点检查：

```text
uk-
Authorization:
signed URL
.env
raw provider responses
user/account identifiers
```

如果 secret 曾进入 Git 历史：

> 仅删除当前文件不够，必须 rewrite history，并 revoke credential。

公开前建议轮换当前 API Key。

---

## 29. P6 实施顺序

严格按以下顺序：

### P6.1 Official fstest

先完成。

### P6.2 修复 Backend contract bugs

直到：

```text
no unexplained failures
```

### P6.3 Repo rename + namespace cleanup

```text
notes-sync
→ rclone-aliyunenterprise
```

### P6.4 CI

PR CI + optional real integration。

### P6.5 Public docs

README / configuration / safety / troubleshooting。

### P6.6 License / Security / Contributing

补齐公开仓库基础文件。

### P6.7 Secret audit

确保仓库和历史安全。

### P6.8 Release Candidate

```text
v0.1.0-rc1
```

用真实 test Drive 回归。

### P6.9 v0.1.0

满足 release gate 后发布。

---

## 30. v0.1.0 验收标准

### Contract

- [ ] official rclone fstest 无 unexplained failure
- [ ] optional capability 与实际实现一致
- [ ] List correctness regression pass
- [ ] catalog durability pass
- [ ] hidden trash semantics pass
- [ ] Move/Copy metadata regression pass

### Integration

- [ ] lsf
- [ ] copy upload
- [ ] copy download
- [ ] sync
- [ ] bisync
- [ ] nested directories
- [ ] rename/move
- [ ] logical delete
- [ ] conflict preservation
- [ ] invalid API Key fail closed
- [ ] transient Provider failure safe

### Engineering

- [ ] repo 名称正确
- [ ] module/binary/backend 命名统一
- [ ] CI pass
- [ ] gofmt/vet/lint
- [ ] pinned rclone
- [ ] reproducible build
- [ ] no secrets

### Open Source

- [ ] README
- [ ] LICENSE
- [ ] SECURITY.md
- [ ] CONTRIBUTING.md
- [ ] CHANGELOG.md
- [ ] issue templates
- [ ] provider quirks docs
- [ ] safety docs
- [ ] troubleshooting docs

### Public usability

新用户无需阅读源码即可：

```text
理解用途
→ 判断自己是否为 Aliyun Drive Enterprise
→ 获取 API Key
→ build/install
→ config
→ lsf
→ copy
→ sync/bisync
→ 理解限制
```

---

## 31. 发布后的维护原则

### 优先级

优先修复：

```text
data correctness
false deletion
wrong object/path
credential leak
catalog corruption
```

性能排在 correctness 之后。

### Provider changes

Aliyun API 改变：

```text
integration test
→ detect
→ adapter update
```

### rclone upgrades

必须经过完整 regression gate。

### Feature requests

判断：

> 是否属于 rclone Backend capability？

如果不是，而是要求修改 sync/bisync 算法或 App orchestration，则 out of scope。

---

## 32. 软件工程原则

> **Backend 负责 Provider 适配，不重新实现 rclone。**

> **Correctness > convenience > performance。**

> **不确定就失败，不猜测删除。**

> **Provider 不支持的能力诚实声明 unsupported。**

> **所有 workaround 都必须有 regression test。**

> **公共 API、配置和文档面向陌生用户，而不是只服务原作者。**

> **代码可以复杂地处理 Provider quirks，但对 rclone 用户暴露的行为必须简单。**

> **任何用户都不应该因为不了解阿里云 API 的缺陷而承担额外的数据损失风险。**

---

## 33. 最终目标

本阶段完成后，本项目应达到：

> **一个独立、标准、可公开发布、可被其他 rclone 用户直接采用的 Aliyun Drive Enterprise Backend。**

它的价值不局限于原作者自己的同步需求，而是：

> **为所有希望使用阿里云盘企业版作为备份、同步、归档或通用 rclone Remote 的用户，提供一个可靠的社区解决方案。**
