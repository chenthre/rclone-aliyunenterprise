# rclone 阿里云盘企业版 Backend 发布前稳定化指导 v4

> 当前阶段：`v0.1.0-rc1` 发布前最后一轮稳定化与公开发布准备  
> 项目：`rclone-aliyunenterprise`  
> 当前状态：P6 已完成；官方 fstest 基本通过；仅剩已知特殊字符编码问题待最终定性；Backend 已具备公开 RC 条件  
> 本阶段目标：**消除最后的 Backend contract 不确定性，验证 catalog 多进程安全，完成最终回归门禁，并将 `v0.1.0-rc1` 公开发布。**
>
> 本文不新增功能，不扩展产品范围，不修改 rclone 同步算法。所有工作都围绕：
>
> 1. Backend contract correctness；
> 2. Provider-specific safety state；
> 3. OSS 发布质量；
> 4. release readiness。

---

## 1. 当前结论

当前已经确认：

```text
Aliyun Enterprise API
        ↓
aliyunenterprise Backend
        ↓
rclone Fs/Object contract
        ↓
copy / sync / bisync
```

整体技术路线成立。

Backend 当前已具备：

- out-of-tree 构建；
- pinned rclone v1.75.1；
- 标准 Backend 注册；
- List / Put / Open / Update / Remove；
- Move / Copy；
- SHA-1；
- Search fallback；
- GetFile authoritative verification；
- durable catalog；
- hidden trash；
- fail-closed；
- fstest；
- real integration test；
- bisync regression；
- CI / OSS docs / security audit；
- `v0.1.0-rc1` tag。

目前不再进行“大功能开发”。

本阶段只解决：

```text
release blockers
contract uncertainty
correctness risks
public release readiness
```

---

## 2. 本阶段最终目标

完成后应该达到：

```text
official fstest
    → no unexplained failure

catalog multi-process safety
    → proven

integration regression
    → pass

bisync regression
    → pass

public GitHub repo
    → available

v0.1.0-rc1
    → public release

real-world soak
    → stable

v0.1.0
    → ready
```

---

## 3. P7.1 —— 最终处理 `FsEncoding/punctuation`

这是第一优先级。

### 3.1 当前现象

fstest 当前唯一未通过项：

```text
FsEncoding/punctuation
```

Aliyun 对包含以下字符的名称存在拒绝：

```text
/
\
```

并返回：

```text
400 InvalidParameter.Name
```

之前暂时分类：

```text
PROVIDER LIMITATION
```

但公开 release 前必须重新验证：

> 是否可以通过 rclone 标准 filename encoding 层透明适配。

### 3.2 目标

判断：

```text
logical rclone path
        ↓
rclone encoder
        ↓
Aliyun-safe physical name
        ↓
Aliyun
        ↓
decode
        ↓
original logical name
```

是否能稳定 round-trip。

如果可以：

```text
当前问题属于 BACKEND MISSING ENCODING
→ 实现
→ fstest 应 PASS
```

如果不能：

```text
PROVIDER LIMITATION
→ 保留
→ 明确文档化
```

### 3.3 实施要求

优先检查 rclone 当前 pin 版本的：

```text
lib/encoder
MultiEncoder
```

以及官方 Backend 如何定义：

```text
encoding
```

配置。

不要自行设计新的 escaping 格式。

必须优先复用 rclone 标准 encoder。

### 3.4 测试范围

至少测试：

```text
/
\
:
*
?
"
<
>
|
控制字符
前导/尾随空格
Unicode
中文
emoji
full-width characters
```

实际测试集以 rclone fstest 为准。

### 3.5 Round-trip invariant

必须满足：

```text
encode(name)
    ↓
provider stores
    ↓
provider returns stored name
    ↓
decode(name)
    ↓
original logical name
```

且：

```text
encode(A) != encode(B)
```

不能产生名称碰撞。

### 3.6 Path separator 特别注意

`/` 在 rclone 中同时承担：

```text
logical path separator
```

角色。

因此必须明确区分：

```text
directory separator
vs
filename literal slash
```

不能通过简单字符串替换处理。

必须使用 rclone encoder 的标准语义。

### 3.7 验收

#### 情况 A：可支持

要求：

- `FsEncoding/punctuation` PASS；
- unit test；
- integration test；
- README 增加 encoding 说明；
- config 中 `encoding` 使用标准 rclone option；
- 默认值经过测试。

#### 情况 B：不能支持

必须：

- 给出明确技术原因；
- 证明 rclone encoder 也无法保证 round-trip；
- `fstest-results.md` 保持 PROVIDER LIMITATION；
- README Known Limitations 说明；
- 不做自定义不安全 hack。

---

## 4. P7.2 —— Catalog 多进程安全

第二优先级。

### 4.1 为什么要验证

Catalog 当前属于：

> Provider Safety State

而不是普通性能 cache。

它用于保证：

```text
Search miss
≠
confirmed deletion
```

因此多个独立 rclone 进程如果共享同一个 Remote：

```text
Process A
Process B
```

都可能：

```text
load catalog
modify
save
```

当前即使有：

```text
atomic rename
```

也只能保证：

```text
文件不会写一半
```

不能自动保证：

```text
A 更新不会被 B 的旧 snapshot 覆盖
```

### 4.2 测试目标

验证：

```text
multiple processes
same domain_id
same drive_id
same remote root
same catalog
```

同时操作时是否出现：

- lost update；
- catalog rollback；
- known-object 丢失；
- corruption；
- wrong identity；
- stale save overwrite。

### 4.3 最小并发实验

建立两个真实独立进程：

```text
Process A
Process B
```

同时针对同一个 test prefix。

测试：

```text
A create A1
B create B1

A list
B list

A update A2
B update B2
```

最后重新启动第三个进程：

```text
Process C
```

检查 catalog 是否同时包含正确 known state。

### 4.4 Stress test

建议：

```text
N = 20~100
```

轮并发操作。

每轮：

```text
spawn A
spawn B
wait
reload catalog
validate
```

目标不是性能，而是发现：

```text
lost update
```

### 4.5 如果发现 lost update

优先采用最小正确方案。

推荐：

```text
cross-process file lock
        ↓
reload latest catalog
        ↓
apply delta
        ↓
atomic save
        ↓
unlock
```

不要：

```text
lock
→ 使用早先内存 snapshot
→ save
```

锁后必须 reload 最新状态。

### 4.6 锁的职责边界

这里的 lock 仅保护：

```text
local provider catalog
```

不是：

```text
bisync distributed lock
remote device lock
cross-device sync coordination
```

不要扩大 scope。

### 4.7 锁实现要求

优先：

- 标准 Go / 成熟跨平台实现；
- Linux/macOS/Windows 可行；
- crash 后锁自动释放；
- 不依赖 Aliyun API。

### 4.8 验收

必须证明：

```text
2+ independent processes
→ no catalog corruption
→ no lost safety state
→ no false deletion caused by catalog race
```

并增加 regression test。

---

## 5. P7.3 —— 完整 Release Gate

完成 P7.1 / P7.2 后，重新跑完整门禁。

顺序：

```text
gofmt
↓
go vet
↓
unit tests
↓
race tests
↓
mock provider tests
↓
official fstest
↓
real integration
↓
sync regression
↓
bisync regression
```

### 5.1 Unit

```bash
go test ./...
```

全部通过。

### 5.2 Race

```bash
go test -race ./...
```

Catalog 并发相关测试必须包含在 race 检查中。

### 5.3 Fstest

要求：

```text
no unexplained failures
```

最终结果只允许：

```text
PASS
SKIP
UNSUPPORTED OPTIONAL
PROVIDER LIMITATION
UPSTREAM
```

不允许：

```text
UNKNOWN FAIL
```

### 5.4 Integration

真实 Aliyun 重新验证：

```text
Mkdir
Put
List
Open
Range
Hash
Overwrite
Move
Copy
Remove
Rmdir
NotFound
Unicode
encoded special chars
nested dirs
```

### 5.5 Sync

至少：

```text
local → remote
remote → local
delete propagation
rename
nested
```

### 5.6 Bisync

永久 regression：

```text
A create → B
B modify → A
delete
rename
nested
conflict
process restart
catalog persistence
Search stale
Move sparse metadata
```

---

## 6. P7.4 —— Public Repository 准备

通过 Release Gate 后再公开。

### 6.1 创建 GitHub repo

目标：

```text
rclone-aliyunenterprise
```

添加 remote：

```bash
git remote add origin <repo>
```

默认 branch：

```text
main
```

### 6.2 Public 前最后 secret audit

重新扫描：

```bash
git grep -n "uk-"
git grep -n "Authorization:"
git log -p
git rev-list --objects --all
```

同时检查：

```text
.env
signed URL
raw provider responses
personal paths
account identifiers
```

### 6.3 API Key 轮换

即使 audit 无泄漏，公开仓库前仍建议：

> 当前测试 API Key rotate 一次，并 revoke 旧 Key。

---

## 7. P7.5 —— 发布 `v0.1.0-rc1`

当前本地 annotated tag 已存在。

公开后：

```bash
git push origin main
git push origin v0.1.0-rc1
```

创建 GitHub Release。

### 7.1 Release Notes

至少包括：

#### Features

- Aliyun Drive Enterprise API Key auth；
- rclone Fs/Object；
- List consistency guard；
- safety catalog；
- hidden trash；
- SHA-1；
- server-side Move/Copy；
- sync/bisync tested。

#### Limitations

- no physical delete；
- no SetModTime；
- no ListR；
- no ChangeNotify；
- eventual consistency；
- catalog behavior；
- Range provider quirk；
- encoding limitation（如仍存在）。

#### Status

```text
Beta / Release Candidate
```

不要写：

```text
production safe
100% data-loss proof
fully stable
```

---

## 8. P7.6 —— RC Soak Test

公开 RC 后，不立刻发 v0.1.0。

### 8.1 时间

推荐：

```text
至少 3~7 天
```

### 8.2 数据

使用：

```text
test / non-critical data
```

### 8.3 持续运行

覆盖：

```text
copy
sync
bisync
多轮修改
新文件
删除
rename
nested
Unicode
中等大小附件
多进程调用
process restart
API Key invalid
网络波动
```

### 8.4 观察指标

记录：

```text
unexpected deletion
catalog error
ambiguous path
stale listing recovery
Range fallback
retry behavior
trash growth
performance anomalies
```

---

## 9. P7.7 —— Issue Triage

RC 期间问题分类：

```text
BACKEND BUG
PROVIDER CHANGE
UPSTREAM RCLONE
DOCUMENTATION
UX
```

阻塞 v0.1.0：

```text
data correctness
false deletion
wrong object/path
catalog corruption
credential exposure
contract violation
```

通常不阻塞：

```text
performance
optional capability
minor docs typo
trash cleanup convenience
```

---

## 10. P7.8 —— v0.1.0 Release Gate

### Contract

- [ ] fstest no unexplained failure
- [ ] encoding 已 PASS 或最终定性
- [ ] catalog multi-process safety proven
- [ ] capabilities honest

### Correctness

- [ ] no known false deletion bug
- [ ] no known wrong-object bug
- [ ] Move/Copy metadata correct
- [ ] hidden trash correct
- [ ] catalog durable

### Regression

- [ ] unit PASS
- [ ] race PASS
- [ ] integration PASS
- [ ] sync PASS
- [ ] bisync PASS

### OSS

- [ ] public GitHub repo
- [ ] CI green
- [ ] README complete
- [ ] SECURITY
- [ ] CONTRIBUTING
- [ ] CHANGELOG
- [ ] LICENSE
- [ ] issue templates

### Security

- [ ] secret audit PASS
- [ ] old test API Key rotated/revoked
- [ ] logs redacted

### Soak

- [ ] RC real usage completed
- [ ] no release-blocking Backend correctness issue

---

## 11. 正式 `v0.1.0`

通过 Gate 后：

```bash
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

发布 GitHub Release。

---

## 12. v0.1.0 后进入维护模式

正式发布后不马上加功能。

先观察：

```text
real users
issues
provider API changes
rclone upgrades
```

### v0.1.x

只做：

```text
correctness bugfix
security fix
provider compatibility fix
documentation correction
```

### v0.2.0 候选

真实需求出现后再考虑：

- Drive interactive selection；
- About / quota；
- better config UX；
- optional trash maintenance tooling；
- wider platform binaries；
- metadata/performance enhancements。

---

## 13. 当前明确不做

```text
Android
iOS
GUI
FUSE
CRDT
distributed bisync lock
custom conflict resolver
rclone fork
automatic cloud GC
true delete workaround
block-level delta
```

---

## 14. Range / Seek 特别说明

当前已发现：

> Aliyun CDN 可能忽略 HTTP Range。

Backend 通过客户端切片满足 rclone contract。

分类：

```text
semantic correctness PASS
performance limitation
```

必须记录在：

```text
docs/provider-quirks.md
```

说明：

> 随机读取大文件时可能导致完整对象下载。

不是 v0.1 blocker。

---

## 15. Catalog 特别说明

公开文档必须明确：

> Catalog 是 Backend listing safety state，不只是 cache。

不要鼓励用户随意删除。

如果删除：

```text
需要 rebuild / safety reinitialization
```

流程写入：

```text
docs/troubleshooting.md
```

---

## 16. Hidden Trash 特别说明

公开文档明确：

```text
rclone-visible delete
≠
Aliyun physical delete
```

删除内容仍会占用空间。

---

## 17. Commit 策略

建议继续保持小步提交：

```text
P7.1 encoding investigation
P7.1 encoding implementation/final classification
P7.2 catalog concurrency tests
P7.2 catalog locking fix
P7.3 release gate results
P7.4 public repo prep
P7.5 rc1 publish
P7.6 soak notes
P7.8 v0.1.0
```

不要做 mega commit。

---

## 18. 文档同步原则

每个行为变化同时更新：

```text
README
provider-quirks
implementation-notes
fstest-results
CHANGELOG
```

如果只改代码不改文档，则任务未完成。

---

## 19. 完成定义

本阶段完成标准不是：

```text
代码能跑
```

而是：

> **一个陌生 rclone 用户可以公开获取、配置和使用该 Backend，并且所有已知 Provider 差异都被正确适配或诚实暴露。**

最终状态：

```text
rclone-aliyunenterprise
Status: v0.1.0
Public
Tested
Documented
Maintained
```

---

## 20. 实施哲学

> **发布前最后阶段优先消除不确定性，而不是增加功能。**

> **Backend contract 能用标准 rclone 机制解决的，不自行发明协议。**

> **Catalog 属于 correctness state，必须像状态数据一样保护。**

> **所有剩余 failure 必须有归因。**

> **Provider limitation 可以存在，但不能伪装成 Backend success。**

> **RC 的目的不是宣传，而是让真实运行暴露最后的问题。**

> **v0.1.0 的核心承诺是：行为可解释、数据语义正确、错误时失败关闭。**
