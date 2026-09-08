# Upstream rclone issues — attribution log

This file tracks rclone bugs/limitations observed while using this backend.
Our policy (see docs/implementation-notes.md §7): **attribute, record, report —
never patch rclone inside the backend.**

## 1. bisync conflict rename: `missing info for "...conflict2"`

- **Observed**: 2026-09-08, rclone v1.75.1, during the double-device bisync PoC.
- **Scenario**: both endpoints modify the same file between syncs
  (`--conflict-resolve none --conflict-loser num`).
  bisync correctly detected the conflict, renamed the Path1 copy to
  `same.md.conflict1`, renamed the Path2 copy to `same.md.conflict2`, then
  failed with:

  ```
  bisync.renames: internal error: missing info for "same.md.conflict2".
  Please report a bug at https://github.com/rclone/rclone/issues
  ```

- **Data safety**: both conflicting versions were preserved on both ends
  (each version existed locally and was uploaded to the remote; only the
  transfer of one renamed copy was aborted). No data loss.
- **Attribution status**: **RESOLVED — BACKEND CONTRACT VIOLATION (fixed)**.
  The PDS `move`/`copy` responses are metadata-sparse; the backend returned that
  sparse data as the moved Object's metadata (missing size/hash/name), so bisync
  could not obtain `conflict2`'s source info. Fix: `Move`/`Copy` refresh via
  `file/get`. Verified: the full double-modified conflict scenario now completes
  with `Bisync successful`, conflict1/conflict2 preserved on both sides, no
  internal error, no data loss. Local↔local never reproduced the error because a
  filesystem Move returns complete metadata.
- **Reproducer plan (experiment A — local↔local, no backend involved)**:
  1. `mkdir pair-a pair-b; rclone bisync pair-a pair-b <flags> --resync --create-empty-src-dirs`
  2. write `f` on both sides with different content
  3. run `rclone bisync pair-a pair-b ...` (no resync)
  4. check whether `bisync.renames: missing info` reproduces with the local
     backend alone.
  - If yes → **UPSTREAM**; file an rclone issue with the reproducer.
  - If no → re-test with `aliyunenterprise` and inspect List/ID/Move/Copy
    semantics for a backend contract violation (experiment B).
- **Workaround (if accepted)**: avoid same-round double-modify, or use a
  conflict-loser setting that does not re-rename downstream copies.