#!/usr/bin/env bash
# Release helper for rclone-aliyunenterprise (P7.4-P7.8).
#
# Public release steps (requires a GitHub remote):
#
#   export GH_REPO_OWNER=<owner>   # e.g. chenthre
#   tools/release.sh rc2           # or: release.sh publish
#
# Commands this script prints/executes:
#   1) git remote add origin git@github.com:$OWNER/rclone-aliyunenterprise.git
#   2) git push origin main
#   3) git push origin --tags
#   4) create a GitHub Release for v0.1.0-rc2 with the notes below
set -uo pipefail
cd "$(dirname "$0")/.."
OWNER="${GH_REPO_OWNER:?set GH_REPO_OWNER (git remote origin will be added)}"
REMOTE="git@github.com:$OWNER/rclone-aliyunenterprise.git"
TAG="${1:-v0.1.0-rc2}"

echo "== 1. remote =="
git remote get-url origin 2>/dev/null || git remote add origin "$REMOTE"
git remote set-url origin "$REMOTE"

echo "== 2. push main + tag =="
git push -u origin main
git push origin "$TAG"

echo "== 3. release notes (create the GitHub Release manually with this body) =="
cat <<'NOTES'
## rclone-aliyunenterprise v0.1.0-rc2

Community out-of-tree rclone backend for Aliyun Drive Enterprise (API Key).
Pinned rclone v1.75.1; binary embeds a normal rclone.

### Features
- Aliyun Drive Enterprise API Key auth (`bj37789.api.aliyunfile.com`)
- Fs: List (consistent listing), NewObject, Put, Mkdir, Rmdir, Purge
  (logical), server-side Copy/Move, SHA-1
- Object: Open (Range/Seek client-side), Update, Remove (hidden trash),
  Hash, ModTime (read-only), ID
- Listing consistency: native list -> search(parent) -> file/get verification
  -> persistent safety catalog (identity-bound, cross-process locked)
- Hidden-trash logical delete (`_aliyunenterprise_rclone_trash`)
- bisync PoC verified: bidirectional, rename, delete propagation, conflict
  preservation, interruption recovery, invalid-key fail-closed
- Standard rclone name encoding (`encoding`, System|BackSlash default);
  names with `/` `\` round-trip (fstest FsEncoding all PASS)
- Official rclone fstest suite green (FsMkdir: zero unexplained failures)
- Command-level gate (tools/gate-runner.sh) 10/10 across runs

### Limitations (honest)
- No physical delete: removals move to hidden trash (manual GC; quota used)
- No SetModTime / ListR / ChangeNotify
- Eventual consistency: new/moved objects may take seconds to appear; the
  backend never treats temporary absence as deletion
- Range reads slice client-side (CDN ignores Range headers)
- Same-directory server-side copy falls back to generic copy
- Catalog is safety state: corruption/wrong-drive/encoding mismatch fail closed
- between-sync visibility of freshly written objects is eventual by design

### Status
Beta / Release Candidate. 3-7 day soak recommended before v0.1.0. See
docs/fstest-results.md, docs/provider-quirks.md, docs/safety.md.
NOTES

echo "Reminder: rotate/revoke the earlier test API Key before/at public release (docs say)."