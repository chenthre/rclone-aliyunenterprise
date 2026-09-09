# Changelog

All notable changes to this project will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/) and
this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Out-of-tree rclone v1.75.1 backend `aliyunenterprise`.
- API Key Bearer auth against `https://<domain_id>.api.aliyunfile.com/v2/*`.
- Fs: List (with catalog-safe listing), NewObject, Put, Mkdir, Rmdir, Purge
  (logical), Copy/Move (server-side), Hashes(SHA-1).
- Object: Open (with client-side Range/Seek), Update, Remove (hidden trash),
  Size, ModTime (unsupported), Hash, ID.
- Listing correctness: native list → search(parent_file_id, recursive=false)
  → per-item `file/get` verification → persistent safety catalog
  (schema v2, identity-bound, atomic+fsync writes, corruption fail-closed).
- Logical delete: `Remove`/`Rmdir`/`Purge` move objects to the provider-private
  `_aliyunenterprise_rclone_trash` folder.
- bisync PoC verified (bidirectional A↔remote↔B, rename, delete propagation,
  conflict preservation, interruption recovery, invalid-key fail-closed).
- Official fstest contract suite wired and green
  (see `docs/fstest-results.md`; only provider-limitation case remains).

### Added
- `encoding` option (rclone standard encoder, default `Standard | BackSlash`)
  with full round-trip for names containing `/`, `\`, and other provider-
  unfriendly characters (P7.1; fstest `FsEncoding` all PASS).

### Fixed
- Move/Copy returned sparse metadata (PDS responses) — refreshed via
  `file/get`; this fixed bisync `missing info for "...conflict2"`.
- `NewCatalog` never loaded the persisted file (process restart lost state).
- Root `file/list` eventual-consistency handled like sub-directories.
- Duplicate same-name children fail closed instead of arbitrary selection.
- Fs contract fixes from fstest: `List` missing dir → `ErrorDirNotFound`;
  `Object.String()` bare remote + nil-safe; NewObject path convention;
  `Hash()` authoritative local SHA-1; `Precision()` → `fs.ModTimeNotSupported`.

### Security
- API keys and signed URLs are never logged or committed; `.env` git-ignored.
- catalog corruption and wrong-drive catalog are rejected (fail closed).

### Known issues
- Aliyun rejects `/` and `\` in file/dir names (`400 InvalidParameter.Name`).
- Physical delete is unavailable to API keys; deleted objects stay in the
  hidden trash and keep consuming storage (manual GC).
- PDS download endpoint ignores Range headers; range reads slice client-side.
- Same-directory server-side copy is refused (403) — rclone falls back to
  generic copy.
- `ModTime` is read-only on provider; consumers should use size+checksum.

## [0.1.0-rc1] — upcoming

First public release candidate. See Unreleased for the feature set.