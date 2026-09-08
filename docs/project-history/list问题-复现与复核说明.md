# 问题报告与复现方法（供独立复核）—— Sync 团队空间子目录 list 恒为空

> 目的：向独立复核 Agent 提供**自包含**的问题描述与可重复的复现方法，供其验证结论。
> 复核对象：阿里云盘企业版（域名 `bj37789`，组织 Chenthre Cloud）团队空间 `Sync` 的 `POST /v2/file/list` 子目录查询异常。
> 状态：已定位为**服务端数据面异常**（证据链见 §5）；已有客户端回退方案（见 §10）。

---

## 1. 问题一句话描述

**在团队空间 Sync（drive_id=101）中，任何子目录执行 `file/list` 均返回空数组，即使该目录确实包含文件（`file/get`、`file/search`、`file/create`、`file/move` 全部证实文件存在且父目录正确）；而同样的 `file/list` 在空间根目录（parent=root）完全正常。**

---

## 2. 环境与凭证（复核者必读）

| 项 | 值 | 备注 |
|---|---|---|
| 产品 | 阿里云盘企业版（PDS 企业版域，service_code=edm） | 非 PDS 开发者版 |
| **domain_id** | `bj37789` | 即企业代码 |
| 数据面端点 | `https://bj37789.api.aliyunfile.com` | 区域 cn-beijing；`bj37789.api.aliyunpds.com` 返回 `NotFound.Domain` |
| **团队空间 Sync** | `drive_id=101`（owner_type=group） | 本问题涉及对象 |
| 企业空间 Chenthre Cloud | `drive_id=2` | API Key 无权限（403 `ForbiddenNoPermission.File`），勿用于本复现 |
| 认证方式 | `Authorization: Bearer uk-...`（api_key） | 密钥在项目 `.env`（`API_KEY=`），**复核时请勿打印/外泄** |
| 当前用户 | `user_id=1aa30d815bc640c995135c400f43914e`（chenthre，超级管理员） | API Key 权限模式=继承登录用户 |
| 本地工具 | aliyun CLI 3.5.0 + `aliyun-cli-pds` 0.9.0（非必需，REST 直连即可） | 复核脚本 `recheck.py` 仅用 Python `requests` + HTTP |

---

## 3. 现象：预期 vs 实际

以「目录 `d/` 内含文件 `f.txt`」为前提（已被 create/get/search 三方证实）：

| 请求 | 预期（开发者版/常理） | 实际（Sync drive 101） |
|---|---|---|
| `POST /v2/file/list {drive_id:101, parent_file_id:"<d>"}` | 返回 `items=[f.txt]` | **`items=[]`** |
| `POST /v2/file/list {drive_id:101, parent_file_id:"root"}` | 返回 root 下所有对象 | 正常（实时，含新对象） |
| `POST /v2/file/get {file_id:f.txt}` | 返回文件元数据 | 正常（`parent_file_id=<d>`） |
| `POST /v2/file/search {query:name match "f.txt"}` | 返回命中 | 正常（约 1 分钟后索引就绪） |
| `POST /v2/file/list_delta {drive_id:101}` | 返回增量 | **`items=[]`**（同异常） |
| `POST /v2/file/create`（建目录/建文件） | 成功 | 正常 |
| `POST /v2/file/move` / `update` / `copy` | 成功 | 正常 |

---

## 4. 一键复现（推荐）

```bash
cd <项目目录>            # 含 .env（API_KEY=uk-...）
python3 recheck.py
```

脚本自包含：每次创建全新随机目录+文件（纯 REST 分片上传）→ 检测子目录 list → 对照 root list / move 双向 → **自动将对象归档到 `Sync/_trash_chenthre_sync`**（企业版无 delete 权限，归档即清理，不产生残留）。

期望输出要点：

```
[阶段A: 环境准备断言]   5 项全部 PASS（创建目录/文件/上传/get 均成功）
[阶段B: 复现检测]       子目录 list 返回条目数 = 0   →  [复现 ✓]
[阶段C: 对照验证]
  [PASS] move 到 root 后，root list 可见该文件
  [复现 ✓] 同一文件 move 回子目录后，list 子目录又变 0 条
  [PASS] get-file 确认文件仍在子目录内
  [PASS] search(parent_file_id, recursive=false) 可枚举直属子项
[复核结论] 复现成功：子目录 file/list 恒为空，root list 与 search/get/move 正常
```

**判定标准**：若阶段B/阶段C 出现上述「复现 ✓」，且阶段A 全 PASS → 问题复现成立。

---

## 5. 手动复现（最小步骤，独立于脚本）

```bash
KEY=$(grep '^API_KEY=' .env | cut -d= -f2)
API=https://bj37789.api.aliyunfile.com
H1="Authorization: Bearer $KEY"; H2="Content-Type: application/json"

# 1) 建目录
DIR=$(curl -sS -X POST $API/v2/file/create -H "$H1" -H "$H2" \
  -d '{"drive_id":"101","parent_file_id":"root","name":"manual_check_dir","type":"folder","check_name_mode":"refuse"}' \
  | python3 -c "import json,sys;print(json.load(sys.stdin)['file_id'])")

# 2) 建文件记录（拿 upload_url/upload_id 后再 PUT 分片 + complete，可参照 recheck.py ②）
#    或更简单：把已存在对象 move 进来 —— 见下：

# 2') 简便替代：先把一个已有文件 move 到新目录（SQL 同 4 的对照逻辑）
#     curl -X POST $API/v2/file/move -H "$H1" -H "$H2" \
#       -d '{"drive_id":"101","file_id":"<任一已有 file_id>","to_parent_file_id":"'$DIR'"}' 

# 3) 复现点：list 该目录（预期正常应含对象，实际返回 items:[]）
curl -sS -X POST $API/v2/file/list -H "$H1" -H "$H2" \
  -d '{"drive_id":"101","parent_file_id":"'"$DIR"'","limit":100}'
#   实际输出: {"items":[],"next_marker":"","punished_file_count":0}

# 4) 对照：list root（应能看到新目录）
curl -sS -X POST $API/v2/file/list -H "$H1" -H "$H2" \
  -d '{"drive_id":"101","parent_file_id":"root","limit":100}'
```

> 完整分片上传的手工版见项目内 `recheck.py`（§2/§3 步骤）与 `strict_probe.py`。

---

## 6. 已做的排查与排除项（证据链）

### 6.1 官方 Skill 对照结论（新增）

用户提供官方 Skill 包 `alibabacloud-pds-intelligent-workspace.zip`（阿里云官方企业网盘技能），逐份对照后**确认：本报告使用的 list 姿势与官方 100% 一致，不存在误用**：

| Skill 文档 | 官方姿势 | 对照结论 |
|---|---|---|
| `references/download-file.md` | `list-file --drive-id <id> --type folder/file --parent-file-id <dir_id> --user-agent ...` 逐层遍历 | 与我方一致；官方同样依赖子目录 list |
| `references/upload-file.md` | 同样的 list-file 遍历；`--parent-file-id` 默认 root；`--check-name-mode` 默认 ignore(覆盖) | 与我方一致 |
| `references/config.md` | 官方默认认证：`ak` 配域 → `list-user` → `token`+`--user-id`（用户身份）；api_key 为另一用户身份路径 | 我方用 api_key（Qoder 文档路径 B） |
| `references/drive.md` | 空间枚举：`list-my-group-drive`（items=团队空间，root_group_drive=企业空间） | 验证可用（返回 Sync drive 101） |
| `references/search-file.md` | 搜索用 `/v2/file/search`（与 list 互补，不替代目录列表） | 无替代姿势 |
| `SKILL.md` | `[MUST]` 每条命令带 `--user-agent`；前置 `ai-mode enable`+`set-user-agent`，结束后 disable | **按此完整重测，仍复现空** |

**按官方姿势重测结果**（含 `--type folder`/`--type file`、`--user-agent`、ai-mode 全流程、`--parent-path`、`api_key+--user-id` 配置、手动改 `token` 认证类型尝试）：子目录 list 依旧空；root list 正常（`--type` 参数在 root 查询中行为验证正确）。

**结论**：非客户端误用；差异仅剩认证面（官方默认 AK/token，本域 `auth_ram_enable=false` 且无 OAuth app，token/AK 路径不可验）。api_key 认证面下子目录 `file/list` 与 `list_delta` 呈服务端数据面异常；`search`/`get`/`create`/`move`/`copy`/`update`/`download` 全部正常。

| # | 排查 | 结果 | 排除/确认 |
|---|---|---|---|
| 1 | CLI `--cli-dry-run` 检查请求体 | `POST /v2/file/list {"drive_id":"101","parent_file_id":"<dir>"}`，与 root 请求完全同构 | **排除**参数/CLI 层问题 |
| 2 | 纯 REST 重建（不经插件，create→upload→complete，见 `strict_probe.py`） | 子目录 list 依旧空；get/root list/search 正常 | **排除**客户端写入路径问题 |
| 3 | 请求形式矩阵（body/URL query/GET/自定义 header/`v2/batch`/vpc 端点） | 全部复现空；附带发现 `v2/batch` 也是 403 | **排除**调用姿势问题 |
| 4 | 目录对象 `get-file` 的 `action_list` | 含 `FILE.LIST`，权限齐备 | **排除**权限问题 |
| 5 | **决定性实验**：同一文件仅改父目录（root⇄子目录移动） | 在子目录时 list 空；move 到 root 后 root list **立即可见**；move 回子目录 **又为空**；get/search 全程正确 | **确认**：list 服务本身工作，仅「子目录作为 parent」查询异常 |
| 6 | 索引/最终一致性等待 | 跨 40+ 分钟多次重试依旧空（root list 反而已实时同步新对象） | **排除**短暂延迟 |
| 7 | 全新目录对照（`_list_probe`、`probe_rest`、recheck 历次运行） | 所有全新目录同样空 | **排除**旧对象/历史脏数据 |

---

## 7. 影响与关联

- **直接影响**：无法用 `file/list` 枚举子目录 → 同步引擎不能依赖列表接口做变更发现；`list_delta` 同样为空。
- **可用替代（已验证）**：`file/search` 用 `parent_file_id = "<id>"` + `recursive:false` 枚举直属子项，再用 `file/get` 单点确认；移动/复制/覆盖/重命名均正常。
- **其他服务端边界（已另行记录，不属本问题**：
  - `file/delete`、`recyclebin/trash`、`v2/batch` → 403 `CheckRouterAccessFailed`（企业版 API Key 无删除/批量能力）
  - 覆盖写不保留历史 revision（域配置 `multi_revision_config.enabled=false`）
  - search 有约 1 分钟索引延迟

---

## 8. 复核检查清单

- [ ] 确认环境与 §2 一致（domain=**bj37789**、端点 `bj37789.api.aliyunfile.com`、drive=101）
- [ ] 运行 `python3 recheck.py`，阶段A 5 项全 PASS
- [ ] 阶段B 输出「子目录 list 返回 0 条」且「复现 ✓」
- [ ] 阶段C 对照链成立（root 可见 / 移回子目录又空 / get-file 确认物理位置）
- [ ] 阶段C 回退验证通过（`search(parent_file_id, recursive=false)` 可见直属子项）
- [ ] （可选）手动 curl 按 §5 复核一次
- [ ] 检查 Sync 根目录：除 `_trash_chenthre_sync` 外无新残留；确认脚本已自动归档
- [ ] 若阶段B 反而返回 1 条 → 服务端已修复，向主 Agent 反馈

## 9. 复核注意事项（安全与规范）

1. **密钥**：`.env` 中 `API_KEY=uk-...` 属敏感信息；复核日志/输出中不得出现明文（recheck.py 已保证）。
2. **禁止删除**：企业版无删除能力；清理一律用「move 到 `Sync/_trash_chenthre_sync`」约定（recheck.py 已自动执行）。
3. **禁止触碰企业空间 drive=2**：当前 Key 无权限（403），且其中为公司现有数据。
4. 如需向阿里云提工单，附本文件 + recheck.py 输出即可自证。

## 10. 已验证的客户端解决方案（2026-09-08）

服务端 `/v2/file/list` 异常目前仍可复现，无法在客户端真正修复该接口；但可以用公开的
`/v2/file/search` 实现语义等价的「列直属子项」：

```json
{
  "drive_id": "101",
  "query": "parent_file_id = \"<目录 file_id>\"",
  "recursive": false,
  "return_total_count": true,
  "limit": 100
}
```

关键点：`recursive` 必须为 `false`。实测 `true` 会把父目录自身和更深层后代混入结果；
`false` 只返回 `parent_file_id` 精确匹配的直属子项。查询字段和语法也已在官方 Skill 的
`get_scalar_query_prompt.py` 中确认。

项目内已新增可复用实现：

```bash
python3 pds_list_children.py \
  --drive-id 101 \
  --parent-file-id '<目录 file_id>' \
  --pretty
```

默认策略为：root 或正常的非空 list 结果继续使用 `/v2/file/list`；子目录 list 异常为空时，
自动回退到 `search(parent_file_id, recursive=false)`，并逐项调用 `file/get` 复核当前位置，
避免搜索索引延迟导致已移动/删除对象误报。脚本支持 marker 分页；单元测试见
`test_pds_list_children.py`。

限制：`search` 仍是最终一致索引，新建对象可能短暂不可见。同步引擎应保留本地状态库并周期
重扫；刚完成写操作时应以 create/complete 返回的 `file_id` 和 `file/get` 为准，不能把一次
空搜索直接解释为远端删除。
