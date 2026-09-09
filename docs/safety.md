# Safety

## Credentials

- Never commit the API key or `.env`.
- Use the least-privileged key (target drive only) with an expiry.
- Rotate as soon as anything looks suspicious.
- Signed download/upload URLs must not appear in shared logs (the backend
  redacts authorization and does not log URLs).

## Catalog (provider safety state)

The catalog at `~/.cache/rclone-aliyunenterprise/catalog.json` is **not a
performance cache**. It is how the backend keeps "this object is known to be in
this folder" state despite Aliyun's eventual-consistent search index.

- Do **not** delete it casually; corruption fails closed (the backend refuses
  to run with an unreadable catalog).
- A catalog from another drive/domain is rejected.
- Removing it means the backend starts with an empty memory and can only
  re-learn state from listings — safest after a full re-list, not as a
  "clearing" ritual during an outage.

## Logical delete (hidden trash)

`delete` / `Remove` / `sync` deletions move objects to the internal folder
`_aliyunenterprise_rclone_trash` at the remote root. Consequences:

- **"Deleted" means hidden from the rclone-visible namespace, not physically
  removed from Aliyun storage.**
- Deleted files still consume drive quota. The v0.1 backend has **no automatic
  GC**; clean up manually in the Aliyun web console when you are sure.
- Never delete the trash folder itself while you still want recoverability.

## Using sync / bisync

- bisync is an **rclone** capability; this backend is tested with it but does
  not re-implement or patch it.
- Cold start: the remote path must exist — `rclone-aliyunenterprise mkdir
  aliyun:vault` before the first bisync `--resync`.
- First run: `--dry-run`, inspect, then `--resync` once; afterwards run normal
  bisync without `--resync`.
- Compare by **size+checksum**: `--compare size,checksum` (the provider cannot
  store client mtimes).
- Keep `--max-delete` conservative; back up important data.
- Eventual consistency means a change can take a few seconds to be visible from
  the other side; **a temporary absence is never treated as deletion** (this is
  the backend's core invariant).

## Bisync flags used in the PoC

```bash
--compare size,checksum --create-empty-src-dirs --resilient --recover \
--max-delete 80 --conflict-resolve none --conflict-loser num --workdir <local>

> `--max-delete N` means **N percent** of files (rclone safety). For small
> vaults use a high value (e.g. 80) or the sync will abort on ordinary
> deletions.
```

## If something looks wrong

- Gather a `-vv` log (sanitized) and classify: backend bug vs provider
  behavior vs rclone upstream (see CONTRIBUTING.md).
- If a directory listing ever seems incomplete, do not sync "as if empty";
  the backend fails closed on unknown state — treat an error, not a guess.