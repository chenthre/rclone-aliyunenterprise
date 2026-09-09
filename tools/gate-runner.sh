#!/usr/bin/env bash
# Command-level release gate for the aliyunenterprise backend (out-of-tree
# equivalent of rclone's fs/operations + fs/sync integration, run against the
# real provider).
#
# Requires env: ALIYUN_ENTERPRISE_API_KEY, ALIYUN_ENTERPRISE_DOMAIN_ID,
#                ALIYUN_ENTERPRISE_DRIVE_ID
# Usage: tools/gate-runner.sh
#
# Semantics notes:
# - The provider is eventually-consistent: every "object must be visible" step
#   polls until visible (wait_visible), and deletions assert *eventual*
#   invisibility with retries.
# - `move` in the rclone CLI treats the destination as a directory; single
#   object rename uses `moveto`.
# - A fresh catalog file is used per run (regression isolation); the empty
#   file is treated as fresh (not corruption).
set -uo pipefail
cd "$(dirname "$0")/.."
RC="${RC:-./rclone-aliyunenterprise/rclone-aliyunenterprise}"
export RCLONE_ALIYUNENTERPRISE_API_KEY="${ALIYUN_ENTERPRISE_API_KEY:?set ALIYUN_ENTERPRISE_API_KEY}"
export RCLONE_ALIYUNENTERPRISE_DOMAIN_ID="${ALIYUN_ENTERPRISE_DOMAIN_ID:?}"
export RCLONE_ALIYUNENTERPRISE_DRIVE_ID="${ALIYUN_ENTERPRISE_DRIVE_ID:?}"
CAT="$(mktemp -t ae-gate-catalog.XXXXXX.json)"
export RCLONE_ALIYUNENTERPRISE_CATALOG_PATH="$CAT"
T="ae-gate-$$"
PASS=0; FAIL=0
ok(){ PASS=$((PASS+1)); echo "  [PASS] $1"; }
bad(){ FAIL=$((FAIL+1)); echo "  [FAIL] $1"; }
step(){ echo "== $1 =="; }

# wait_visible <remote-dir> <name> — poll lsjson until the name appears (final consistency)
wait_visible(){
  local dir="$1" name="$2"
  for _ in $(seq 1 20); do
    "$RC" lsjson "$dir" 2>/dev/null | grep -q "\"Name\":\"$name\"" && return 0
    sleep 1
  done
  return 1
}

cleanup(){ "$RC" purge ":aliyunenterprise:$T" >/dev/null 2>&1; rm -f "$CAT" "$CAT.lock"; }
trap cleanup EXIT

step "lsf root"
"$RC" lsf ":aliyunenterprise:" >/dev/null 2>&1 && ok "lsf" || bad "lsf"

step "mkdir + copy upload (nested, unicode)"
mkdir -p /tmp/ae-gate-src/嵌套/"a b"; echo hi > /tmp/ae-gate-src/嵌套/"a b"/"c!.txt"; echo x > /tmp/ae-gate-src/plain.txt
"$RC" mkdir ":aliyunenterprise:$T" >/dev/null 2>&1
"$RC" copy /tmp/ae-gate-src ":aliyunenterprise:$T" >/dev/null 2>&1 && ok "copy upload" || bad "copy upload"
wait_visible ":aliyunenterprise:$T" "plain.txt" \
  && ok "upload visible" || bad "upload never visible"

step "check (checksum verify)"
"$RC" check /tmp/ae-gate-src ":aliyunenterprise:$T" --checksum >/dev/null 2>&1 && ok "check" || bad "check"

step "copy download + byte compare"
rm -rf /tmp/ae-gate-dst
"$RC" copy ":aliyunenterprise:$T" /tmp/ae-gate-dst >/dev/null 2>&1
diff -r /tmp/ae-gate-src /tmp/ae-gate-dst >/dev/null 2>&1 && ok "download identical" || bad "download identical"

step "special-char filename round-trip"
printf 's' > /tmp/ae-gate-s.txt
"$RC" copyto /tmp/ae-gate-s.txt ":aliyunenterprise:$T/s!name" >/dev/null 2>&1
wait_visible ":aliyunenterprise:$T" "s!name" && ok "special-char round-trip" || bad "special-char round-trip"

step "server-side move/rename (moveto: cross-dir + rename)"
"$RC" mkdir ":aliyunenterprise:$T/sub" >/dev/null 2>&1
"$RC" moveto ":aliyunenterprise:$T/plain.txt" ":aliyunenterprise:$T/sub/moved.txt" >/dev/null 2>&1
if wait_visible ":aliyunenterprise:$T/sub" "moved.txt"; then
  ftype=$("$RC" lsjson ":aliyunenterprise:$T/sub/moved.txt" 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); print([i.get("IsDir") for i in (d if isinstance(d,list) else [d])])')
  [ "$ftype" = "[False]" ] && ok "move+rename (IsDir=False)" || bad "move+rename: moved.txt not a file"
else
  bad "move+rename: moved.txt never became visible"
fi

step "logical delete (Remove -> hidden trash, not listed)"
if wait_visible ":aliyunenterprise:$T/sub" "moved.txt"; then
  deleted=0
  for _ in $(seq 1 10); do
    if "$RC" deletefile ":aliyunenterprise:$T/sub/moved.txt" >/dev/null 2>&1; then deleted=1; break; fi
    sleep 2
  done
  [ "$deleted" = 1 ] && ok "deletefile succeeded" || bad "deletefile never succeeded"
else
  bad "delete: object never became visible"
fi
"$RC" lsjson ":aliyunenterprise:$T/sub" 2>/dev/null | grep -q '"Name":"moved.txt"' && bad "delete visible" || ok "delete hidden"

step "re-download after delete (eventually NotFound)"
gone=0
for _ in $(seq 1 10); do
  if ! "$RC" cat ":aliyunenterprise:$T/sub/moved.txt" >/dev/null 2>&1; then gone=1; break; fi
  sleep 1
done
[ "$gone" = 1 ] && ok "NotFound on deleted" || bad "deleted object still readable after 10s"

echo
echo "gate summary: ${PASS} pass, ${FAIL} fail"
exit "$FAIL"