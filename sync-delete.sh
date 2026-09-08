#!/usr/bin/env bash
# 阿里云盘企业版 Sync 空间「逻辑删除」工具
# 背景：企业版 API Key 无 delete/trash 权限（403 CheckRouterAccessFailed），
#       约定「删除」= 将对象移动到 Sync/_trash_chenthre_sync/（根目录归档夹）。
# 用法：
#   sync-delete.sh <file_id> [--name <重命名建议>]
#   sync-delete.sh --recover <file_id>          # 从 trash 移回根目录（恢复）
#   sync-delete.sh --list-trash                 # 用 search 列举 trash 内容（list 接口不可靠）
# 密钥从 .env 读取（API_KEY=...），不落盘、不打印。
set -uo pipefail
cd "$(dirname "$0")"
ENV_FILE=".env"
ALIYUN="${ALIYUN:-$HOME/.local/bin/aliyun}"
DRIVE_ID="${DRIVE_ID:-101}"
TRASH_NAME="_trash_chenthre_sync"

if [[ ! -f "$ENV_FILE" ]]; then echo "缺少 $ENV_FILE" >&2; exit 1; fi

# 解析 trash 目录 ID（不存在则创建）
resolve_trash() {
  local LIST
  LIST=$("$ALIYUN" pds list-file --drive-id "$DRIVE_ID" --parent-file-id root 2>/dev/null)
  local ID
  ID=$(echo "$LIST" | python3 -c "
import json,sys
try:
    d=json.load(sys.stdin)
    for i in d.get('items',[]):
        if i.get('type')=='folder' and i.get('name')=='$TRASH_NAME':
            print(i['file_id']); break
except Exception: pass
")
  if [[ -z "$ID" ]]; then
    echo "创建 $TRASH_NAME ..." >&2
    ID=$("$ALIYUN" pds create-file --drive-id "$DRIVE_ID" --type folder \
        --name "$TRASH_NAME" --parent-file-id root --check-name-mode refuse \
        | python3 -c "import json,sys; print(json.load(sys.stdin)['file_id'])")
  fi
  echo "$ID"
}

case "${1:-}" in
  --recover)
    TRASH=$(resolve_trash)
    "$ALIYUN" pds move-file --drive-id "$DRIVE_ID" --file-id "$2" --to-parent-file-id root
    ;;
  --list-trash)
    TRASH=$(resolve_trash)
    "$ALIYUN" pds search-file --drive-id "$DRIVE_ID" --name "$TRASH_NAME" 2>/dev/null | python3 -c "
import json,sys
d=json.load(sys.stdin)
# search 不直接按 parent 过滤；用全局扫描代替
" 
    # 用全局 search 列举，判断 parent 为 trash
    KEY=$(grep '^API_KEY=' "$ENV_FILE" | cut -d= -f2)
    curl -sS -m 15 "https://bj37789.api.aliyunfile.com/v2/file/search" \
      -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
      --data-binary '{"drive_id":"'$DRIVE_ID'","query":"name < \"zzzzzzzz\"","limit":100}' \
      | python3 -c "
import json,sys
TRASH='$TRASH'
d=json.load(sys.stdin)
for i in d.get('items',[]):
    if i.get('parent_file_id')==TRASH:
        print(f\"{i['file_id']}  {i['type']:6} {i['name']}  size={i.get('size')}\")
"
    ;;
  *)
    if [[ $# -lt 1 ]]; then echo "用法见文件头注释" >&2; exit 1; fi
    TRASH=$(resolve_trash)
    NAME_OPT=()
    if [[ "${2:-}" == "--name" && -n "${3:-}" ]]; then NAME_OPT=(--check-name-mode auto_rename); fi
    "$ALIYUN" pds move-file --drive-id "$DRIVE_ID" --file-id "$1" --to-parent-file-id "$TRASH" "${NAME_OPT[@]}"
    ;;
esac