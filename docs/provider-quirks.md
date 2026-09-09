# Provider quirks — Aliyun Drive Enterprise (verified against bj37789, 2026-09)

Facts learned from live experimentation on a real enterprise account. These
explain the workarounds inside the backend and are also useful for writing
support tickets with Alibaba Cloud.

## 1. Endpoints & auth

- Data plane: `https://<domain_id>.api.aliyunfile.com/v2/*` (cn-beijing gateway).
- Auth: `Authorization: Bearer <uk-... enterprise api key>`; no RAM AK, no OAuth
  token path on this account (`auth_ram_enable=false`, no OAuth app).
- `*.api.aliyunpds.com` → `NotFound.Domain`; bare domains don't resolve.

## 2. Sub-directory listing defect

- `file/list` on the drive **root** works.
- `file/list` on **any sub-directory** returns `items:[]` (and `next_marker` empty).
- `file/list_delta` is also empty at sub-directory level.
- Same objects are reachable via `file/get`, `file/search`, `create`, `move`,
  `copy`.
- Search with `query: parent_file_id = "<parent>"` and `recursive: false` returns
  exactly the direct children (verified 13/13 objects on a real folder).

Workaround in backend: list → search(parent) → per-item get verification.

## 3. Search is eventual-consistent

- Newly uploaded/moved objects may be invisible to search for **seconds** and
  during renames/moves the old parent may be reported briefly.
- Consequence: a sync consumer must never interpret a search miss as deletion
  (backend safety catalog + `file/get` re-verification implement this).

## 4. No real delete

- `DELETE` file, `trash-file`, `v2/batch` → `403 CheckRouterAccessFailed`.
- Domain config: `recycle_bin_config.delete_trash_normal_file_disabled=true`.
- Workaround: logical delete = move to private trash folder; gc is manual.

## 5. Hashes & overwrite

- `content_hash` is SHA-1, returned **uppercase** hex (normalized to lowercase).
- Overwrite-in-place (create with existing `file_id`) keeps the same `file_id`
  and bumps `revision_id`, but the domain has `multi_revision_config.enabled=false`
  so old revisions 404 (`NotFound.Revision`) — no history.

## 6. ModTime

- `updated_at` is readable (with ms precision) but cannot be set arbitrarily.
- Backends must not fake mtimes; consumers should compare size+checksum.

## 7. Enterprise space permissions

- The api_key sees only its granted team space (drive 101 here);
  `list-my-group-drive` returns `[("101","Sync")]`, `root_group_drive=None`;
  the enterprise space (drive 2) returns `403 ForbiddenNoPermission.File`.

## 8. Domain config snapshot (bj37789, 2026-09)

```
multi_revision_config.enabled=false
recycle_bin_config.delete_trash_normal_file_disabled=true
user_api_key_config.enabled=true (quota=2)
service_code=edm
spi_instance_id=pds_trc_public_cn-xpz4wo1t701
path_type/mode=StandardMode
data_hash_name=sha1
```
## 9. Fresh-object convergence caveats (P5 observations)

- Root `file/list` is also eventual-consistent for just-created objects (sub-second to
  a few seconds), not just `file/search`. Any "existence check" must be treated as
  racy and combined with `file/get` + local catalog state.
- Concurrent creation of the same directory name on one account can race the index;
  proper backends should detect duplicates and fail closed rather than guess.

## 10. P6.1 contract-test discoveries (2026-09-09, live fstest run)

- **File/dir names** must not contain `/` or `\` (server `400 InvalidParameter.Name:
  should not contain \/`). Other punctuation, spaces, unicode, control chars,
  and URL-encoding are accepted.
- **Range reads**: the CDN download endpoint ignores `Range` headers (returns
  the full body even for `bytes=81-100000`); backends must slice client-side.
- **Same-directory server-side copy** returns `403 ForbiddenNoPermission.File`;
  copying to another directory works.
- **ModTime**: `updated_at` is read-only; `SetModTime` is unsupported
  (`fs.ModTimeNotSupported`).
- **Move/Copy API** accept only a target parent (no target name); renaming is a
  separate `file/update` call. (Fully handled inside the backend.)
