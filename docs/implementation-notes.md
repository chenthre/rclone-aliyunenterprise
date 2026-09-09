# Implementation notes — aliyunenterprise backend

Version: v2 (2026-09) · rclone pinned: v1.75.1

This document records the architecture decisions and safety invariants of the
`aliyunenterprise` backend. It is the reference for why the code is structured
the way it is.

## 1. Why an out-of-tree backend

- rclone has no in-tree Aliyun Drive backend (checked v1.62–v1.75).
- Out-of-tree mode (import `rclone/cmd` + own `fs.Register`) requires **no rclone
  source changes** and stays buildable as a normal Go module.
- Future consumers (CLI, bisync, librclone, gomobile/AAR) can embed the same
  compiled binary / module. No Linux-only plugin is used.

## 2. List() correctness is the primary safety constraint

rclone/bisync treat `Fs.List()` output as the trusted set of direct children.
If a temporarily-missing item were reported as absent, a sync run could
propagate deletions. Therefore:

1. Native `file/list` is used for root (known-good) and any non-empty result.
2. Empty sub-directory lists are treated as suspect (service defect) and the
   backend falls back to:
   `file/search(parent_file_id=<id>, recursive=false)` (all pages),
   then **`file/get` per item** to verify the authoritative parent/status.
3. A persistent **safety catalog** (local JSON) remembers known objects:
   a known object that search misses is re-verified via `file/get` and only
   removed from listings when the provider authoritatively 404s.
4. **Fail closed**: repeated search failure, pagination loops, or a cascade of
   `get` errors cause `List()` to return an error — never a half-empty listing.

Invariant: **search absence ≠ confirmed deletion**.

## 3. Safety catalog

- Fields: `file_id`, `parent_file_id`, `name`, `size`, `sha1`, `updated_at`,
  `last_seen_at` per directory snapshot; plus schema version, domain/drive id,
  and remote root for identity.
- Persistence: atomic write (tmp + fsync + rename) with a `.bak` file, schema
  version, corruption detection, and wrong-drive rejection (P5.2).
- Scope: provider cache only. It is not exposed to rclone and is not business data.

## 4. Logical delete

No DELETE/TRASH capability for api_key (`403 CheckRouterAccessFailed`), so:

- `Object.Remove()` / `Fs.Rmdir()` / `Fs.Purge()` move objects into a
  provider-private folder `_aliyunenterprise_rclone_trash/<timestamp>-<file_id>-<name>`
  (auto_rename on conflicts).
- The trash folder is filtered out of every listing and is never returned by
  `NewObject`; objects inside trash never re-enter the normal remote namespace.
- Physical GC is a non-goal (admin can clean via web console).

## 5. Honest capability declaration

| Capability | Declaration | Reason |
|---|---|---|
| SHA-1 hash | yes | provider `content_hash` (hex upper→lower normalized) |
| ModTime | read-only | `SetModTime` returns unsupported; never fake a mtime DB |
| ListR | no | search final-consistency is not a reliable recursive-listing contract |
| server-side Move/Copy | yes | verified; used by rclone as optional features |
| Purge | logical | moves the whole directory tree to hidden trash (matches rclone contract for this backend) |
| real delete / GC | no | provider capability missing |

## 6. Error mapping

Provider errors are typed (`ErrAuth`, `ErrPermission`, `ErrNotFound`,
`ErrRateLimited`, `ErrTransient`, `ErrConsistency`, `ErrProtocol`, `ErrCatalog`)
and classified: 401/403 → permission, 404 → not found, 429 → rate limited,
5xx → transient. Unknown-but-destructive states must surface as errors, not as
"missing".

## 7. What is intentionally NOT here

- rclone internals (bisync baselines, locks, conflict engine) — upstream.
- Any app facade / SyncService / Android code — consumers only.
- Workarounds that alter rclone's own semantics to hide provider quirks.
## 8. P5 corrections & new facts (2026-09-08)

- **The drive-root `file/list` is eventual-consistent too.** Live verification
  shows freshly created objects can be briefly missing from the root listing.
  `childrenOf` therefore runs the *full* list → search → get-verify → catalog
  reconcile path for every parent, including root (no more root special-case).
- **Concurrent duplicate-directory creation.** bisync may create two Fs
  instances for the same remote; during the provider convergence window both can
  create the same-named directory (`check_name_mode=refuse` cannot see the
  other). The backend therefore fails closed on multiple same-named children
  (`ErrConsistency`) instead of picking one arbitrarily.
- **Move/Copy must refresh metadata via `file/get`.** The PDS move/copy
  responses are sparse (file_id/file_name/updated_at only). Returning them as
  the Object metadata broke bisync conflict handling (`missing info for
  "...conflict2"` — a backend contract violation, since fixed).
- **Catalog is schema v2 with identity binding** (provider/domain/drive/root),
  atomic+fsync writes with a `.bak`, corruption detection, and wrong-drive
  rejection — see catalog.go and its unit tests.

## 9. P6.1/P6.2 contract fixes (2026-09-09, live fstest)

- List of missing dir → `fs.ErrorDirNotFound` (contract); bisync cold start
  requires pre-creating the remote path (see docs/safety.md).
- Object paths are always relative to the Fs root; drive-absolute paths are
  used only internally (backing fixes for NewObject/Move/Copy/Update).
- Move decisions come from an authoritative `file/get`; same-dir move = rename
  via `file/update`; cross-dir move + optional rename via `file/update`.
- Same-dir Copy returns `fs.ErrorCantCopy` (provider 403) — rclone falls back
  to generic copy.
- Hash() returns the locally verified SHA-1 of the uploaded bytes.
- `Precision()` returns `fs.ModTimeNotSupported` — consumers should use
  `--compare size,checksum`.
- Range/Seek reads are sliced client-side (full download + discard) because the
  PDS CDN ignores Range.
- These fixes are pinned by the fstest suite in docs/fstest-results.md.

## 10. Name encoding (P7.1)

Added the standard rclone `encoding` option (`encoder.MultiEncoder`, default
`Standard|EncodeBackSlash`). Logical paths from rclone are per-segment encoded
to provider names (`FromStandardName`) on create/resolve and decoded back
(`ToStandardName`) on listing. This closes the last fstest gap
(`FsEncoding/punctuation` now PASS).

## 11. Catalog multi-process safety (P7.2)

All catalog mutations (Update/Keep/Remove) now run under a cross-process
exclusive lock (`gofrs/flock` on `catalog.json.lock`) with a **reload-then-delta**
pattern: hold the lock → reload the latest persisted state → apply the change →
atomic save → unlock. This prevents lost updates when two independent rclone
processes share one catalog (same domain/drive/root). Crash-safe (OS lock auto
released). Verified by a real two-OS-process helper test and under -race; the
same test fails without the lock (lost update).

## 12. Name encoding in catalog identity (RC2 preflight)

The catalog stores provider-physical names, so the `encoding` option is now
part of the catalog identity: reopening the same catalog with a different
encoding fails closed (TestWrongEncodingRejected); legacy catalogs (no
encoding field) adopt the current encoding on first rebind.
