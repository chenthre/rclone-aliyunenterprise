# Troubleshooting

## 401 AccessTokenInvalid / Auth errors

- Your API key is invalid, expired, or revoked. Re-create it in the console.
- Check the domain id matches the key's domain. Keys are domain-scoped.

## 403 CheckRouterAccessFailed (delete/trash/batch)

- Aliyun Enterprise API Keys do not get real delete/trash routes.
  This backend implements deletion as a move to the hidden trash, so end users
  should not normally see this.
- If a delete fails with this code, the object may be a directory with
  restrictions; try `rclone move` the directory instead, or clean up via the
  web console.

## 403 ForbiddenNoPermission.File

- The key lacks access to that drive/folder (e.g. another drive_id, or the
  enterprise space). Double-check `drive_id`.
- **Same-directory server-side copy** is refused by the provider (known
  quirk): rclone automatically falls back to a generic copy — expect
  read+write traffic, not an error.

## listing consistency check failed / duplicate-name errors

- Two provider objects share a name under the same parent (e.g. leftover
  directories after interrupted runs). The backend refuses to guess.
  Inspect and clean duplicates in the web console, then retry.
- This can also fire when the search index is briefly out of sync right after a
  create; retrying usually succeeds.

## Search index / eventual-consistency delays

- Fresh objects may take a few seconds to appear in listings. Wait and re-list
  before concluding something is missing.
- Moves/renames may briefly report the old parent. The backend re-verifies
  with `file/get`; a single glimpse of an old location is not a deletion.

## catalog corruption / wrong-drive catalog

```
aliyunenterprise: catalog unreadable / catalog belongs to ...
```

- The catalog is safety state; read-only operations refuse to start if it is
  corrupt or mismatched (fail closed).
- Fix: move the bad file aside (`catalog.json` + `.bak`) and let the backend
  recreate it. Do this only when you can afford a cold start (full re-list);
  never delete it as a routine.

## bisync requires resync / empty-listing abort

- Cold start: create the remote dir first (`rclone-aliyunenterprise mkdir
  aliyun:vault`), then run `--resync --dry-run`, inspect, then real `--resync`.
- "Cannot sync to an empty directory" = your local side looks empty while the
  remote has state (or vice versa). Investigate before `--force`.
- Interrupted runs leave a lock file; remove it only when you are sure no other
  bisync is running.

## ModTime differences / re-upload storms

- The provider cannot set modification times; rclone may want to re-upload to
  update mtimes. Use `--compare size,checksum` (bisync) or `--no-update-modtime`
  (copy/sync).

## Range reads slower than expected

- The PDS download endpoint ignores `Range` headers; range reads are sliced
  client-side after a full download. Correct but heavier on network.

## Names with `/` or `\` rejected (400 InvalidParameter.Name)

- Aliyun Enterprise disallows `/` and `\` in file/dir names. Rename the
  object; there is no backend workaround.

## Reporting

- Collect `-vv` output with the API key and signed URLs removed.
- Classify: backend bug (here), provider limitation (here, provider template),
  rclone upstream (rclone issue).
- Never paste keys or full signed URLs.