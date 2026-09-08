# bisync P0–P4 PoC 验证结果（2026-09-08）

> 现归档至 `docs/project-history/` 环境保留；当前 Backend 状态见 `docs/fstest-results.md` 与 `docs/implementation-notes.md`。

> 对应 `docs/project-history/Chenthre Sync rclone接入实施指导 v1.md` 的 P0–P4 与 §15 验证矩阵（该文档因 v2 已归档；当前有效指导见 `docs/planning/`）。
> 结论：**ACCEPT WITH LIMITATIONS**（详见 §6）。决定正式采用
> `rclone bisync + aliyunenterprise out-of-tree Backend` 作为 Chenthre 文件同步第一版技术路线。

---

## 1. 环境基线（pinned）

| 项 | 值 |
|---|---|
| rclone | **v1.75.1-DEV**（pinned `github.com/rclone/rclone v1.75.1`） |
| Go | go1.27.1 linux/amd64 |
| 构建方式 | out-of-tree backend，`go build ./cmd/rclone-aliyunenterprise`（不修改官方源码，import `rclone/cmd` + `cmd/all` + 自研 backend） |
| Backend 名 | `aliyunenterprise`（module `github.com/chenthre/rclone-aliyunenterprise`） |
| 二进制 | `rclone-aliyunenterprise/rclone-aliyunenterprise`（101MB，静态） |
| 远端 | `:aliyunenterprise:`（env: ALIYUN_ENTERPRISE_API_KEY / _DOMAIN_ID / _DRIVE_ID） |
| 认证 | `Authorization: Bearer <uk-...>` / `https://bj37789.api.aliyunfile.com` |
| 配置注入 | 环境变量（不落盘、不打印明文） |

## 2. Backend 实现要点（对应指导 §6-§8）

- `Fs`: NewFs / List / NewObject / Put / Mkdir / Rmdir / Purge（原生逻辑删除）+ 可选 Copy / Move（服务端）
- `Object`: Open / Update / Remove / Size / ModTime / Hash(SHA-1) / ID
- List 一致性（指导 §7）：
  1. 原生 `file/list`：root 或非空直接返回；
  2. 子目录空 → `file/search(parent_file_id=<id>, recursive=false)` 全分页；
  3. **search 命中的每条再 `file/get` 权威复核 parent/status**（修掉服务端索引 stale 的假阳性）；
  4. 持久化 known-object catalog（本地 JSON，`~/.cache/rclone-aliyunenterprise/catalog.json`）：search miss 的已知对象用 `get` 验证，**search 缺失≠删除**；
  5. get 级联失败 >5 → **fail closed**（返回错误，不返回半空列表）。
- Hash：SHA-1（provider `content_hash`，统一转小写 hex 与 rclone 对比一致）。
- 删除（指导 §8）：Remove/Rmdir/Purge = **move 到 provider-private trash**（`_aliyunenterprise_rclone_trash`，List 隐藏）。无真删。
- ModTime：provider updated_at；SetModTime 不支持（`fs.ErrorCantSetModTime`），同步依赖 size+checksum。

## 3. P0–P4 验收

| 阶段 | 验收项 | 结果 |
|---|---|---|
| P0 | `rclone-aliyunenterprise version` | ✅ v1.75.1-DEV |
| P0 | backend 注册可识别 | ✅ `rclone backend list` / `help backend aliyunenterprise` 可见 |
| P0 | 不需要修改官方源码 | ✅ |
| P1 | root list | ✅ |
| P1 | 上传（中文/空格/Unicode/零字节/700KB 二进制/嵌套目录） | ✅ |
| P1 | 下载回环（diff -r 完全一致 / SHA-256） | ✅ |
| P1 | 覆盖更新（同 file_id） | ✅ |
| P1 | 子目录 list（search fallback + get 复核） | ✅ 调试日志确认 fallback 触发 |
| P1 | 逻辑删除（Remove → trash，List 不见） | ✅ |
| P2 | 搜索 miss 不误判删除（catalog + get） | ✅ 删除后无假阳性残留 |
| P2 | fail closed（认证失败/错误级联） | ✅ |
| P4 | 首次 resync dry-run + 真实 resync | ✅ 6 文件全量 |
| P4 | A 新建 → B 收敛（SHA-256 一致） | ✅ |
| P4 | B 修改 → A 更新 | ✅ |
| P4 | 删除传播（两端消失，远端进 trash） | ✅ |
| P4 | 重命名（A→B，内容保留） | ✅ |
| P4 | 嵌套目录往返 | ✅ |
| P4 | 冲突不丢版本 | ✅ 双版本双副本 |

## 4. §15 安全矩阵

| 场景 | 结果 | 说明 |
|---|---|---|
| 15.1 双向新建 | ✅ | device-a→device-b 内容逐字节一致 |
| 15.2 单端修改 | ✅ | B→A 内容更新 |
| 15.3 目录嵌套 | ✅ | todo/nested/idea.md 往返 |
| 15.4 重命名/移动 | ✅ | 表现为 copy+removal，最终内容正确 |
| 15.5 删除 | ✅ | 用户 namespace 不可见；trash 保留 |
| 15.6 冲突 | ✅（数据安全） | 双方版本都保留；rclone bisync 自身触发内部 bug（见 §5.3），无数据丢失 |
| 15.7 search 索引延迟 | ✅ | 新对象 short window 延迟发现；已知对象绝不误删 |
| 15.8 网络失败 | ✅（设计） | 瞬时错误重试；级联/认证失败 fail closed |
| 15.9 进程中断 | ✅ | kill -9 后 lock 文件拒绝并发；删 lock 后恢复，数据完整 |
| 15.10 API Key 失效 | ✅ | `401 AccessTokenInvalid` 明显失败，0 破坏 |
| 15.11 空 listing 安全 | ✅ | 不存在目录返回空列表；未知状态不触发删除 |

## 5. Provider quirk 记录（服务端/行为）

1. **子目录 `file/list` 与 `list_delta` 恒空**（root 正常）→ search fallback 是唯一可靠枚举路径。
2. **`file/search` 最终一致**：刚上传/移动的对象最长可见数秒缺失；Move 后有短暂 stale-parent 窗口 → 必须以 `file/get` 复核 parent。
3. **无删除能力**：`delete/trash/batch` → 403；逻辑删除依赖 move 到 trash；trash 内同名冲突自动 `_YYYYMMDD_HHMMSS` 后缀。
4. **SHA-1 为大写 hex**（标准化为小写映射到 rclone）。
5. **ModTime 不可设置**：必须 `--compare size,checksum`（bisync）或 `--no-update-modtime`（copy），否则每次运行都会 replace。
6. **覆盖写无历史**：域 `multi_revision_config.enabled=false`；早期 revision `404 NotFound.Revision`。
7. **认证**：仅 Bearer api_key；`*.api.aliyunpds.com` → `NotFound.Domain`；无 RAM AK / OAuth token 路径。

## 6. 结论：ACCEPT WITH LIMITATIONS ✅

采用 `rclone bisync + aliyunenterprise Backend` 作为 Chenthre 文件同步第一版技术路线。

可接受限制（数据安全不受影响、已文档化、对用户可解释）：

1. **新远端对象延迟发现**：search 索引收敛前（秒级）本设备一轮可能看不到对端新文件，下一轮收敛。**绝不误报删除**。
2. **隐藏 trash 需要人工周期清理**：删除 = 移入 `_aliyunenterprise_rclone_trash`，无自动 GC；管理员可 Web 手工清理或在未来开通 delete 权限后切换真删除。
3. **部分 rclone optional capability 不支持**：ListR / ChangeNotify / Purge(原生已实现) / dir modtime / E2EE 等未暴露（nil feature → rclone 自动回退）。

## 7. 遗留风险 / 上游 bug / 下一步

1. **rclone bisync 内部 bug**（①-reported, rclone 1.75.1）：
   `bisync.renames: internal error: missing info for "same.md.conflict2". Please report a bug` —— 冲突双端重命名场景触发，**无数据丢失**（双方版本在各自端+远端各双副本），但 conflict2 副本未下载到对端。需在 rclone 上游 issue 跟踪（建议用最小本地场景复现后上报）。
2. **P3 rclone 官方 contract 测试未全套执行**：本次用真实场景验证；建议后续跑 `fstest`/`integration`（官方 `fs/test` 框架）加速集成为 rclone 上游的准备。
3. **Android**：librclone/gomobile AAR 构建路径需单独 spike（不在本轮）。
4. **生产化清单**：Sync 专用最小权限 Key（仅 drive 101 + 有效期）；`--check-access`（各设备放 RCLONE_TEST 文件）；工作目录/锁；与 `_trash_chenthre_sync` 的归档约定统一。

## 8. 复现/回归命令

```bash
export ALIYUN_ENTERPRISE_API_KEY=$(grep '^API_KEY=' .env | cut -d= -f2)
export ALIYUN_ENTERPRISE_DOMAIN_ID=bj37789  ALIYUN_ENTERPRISE_DRIVE_ID=101
./rclone-aliyunenterprise/rclone-aliyunenterprise lsf ":aliyunenterprise:" -R
./rclone-aliyunenterprise/rclone-aliyunenterprise bisync testdata/device-a ":aliyunenterprise:bisync-test" \
  --compare size,checksum --create-empty-src-dirs --resilient --recover \
  --max-delete 20 --conflict-resolve none --conflict-loser num --resync --workdir /tmp/bisync-work
```