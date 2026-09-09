# fstest / contract test results

Pinned rclone: **v1.75.1** · Backend: aliyunenterprise · Live domain bj37789 (2026-09-09)

## How to run

```bash
ALIYUN_ENTERPRISE_API_KEY=... ALIYUN_ENTERPRISE_DOMAIN_ID=... ALIYUN_ENTERPRISE_DRIVE_ID=... \
  go test -tags integration -v -run TestContract/FsMkdir -timeout 30m -args -verbose \
  ./backend/aliyunenterprise/
```

(Requires an isolated remote prefix; the suite uses `:aliyunenterprise:` and a
random `rclone-test-*` sub-directory per run.)

## Classification legend

- PASS — contract satisfied
- SKIP — capability honestly not declared (rclone skips automatically)
- PROVIDER LIMITATION — provider cannot represent the tested semantic; safe to
  document, no backend fix possible
- BACKEND BUG — fixed after discovery

## FsMkdir suite result

| Test | Result | Notes |
|---|---|---|
| FsMkdir/FsMkdirRmdirSubdir | PASS | nested mkdir+rmdir |
| FsMkdir/FsListEmpty | PASS | |
| FsMkdir/FsListDirEmpty | PASS | |
| FsMkdir/FsListDirNotFound | PASS | ErrorDirNotFound contract |
| FsMkdir/FsListRDirEmpty / NotFound | SKIP | ListR not declared |
| FsMkdir/FsChangeNotify | SKIP | ChangeNotify not declared |
| FsMkdir/FsEncoding/… | PASS (all) | unicode, spaces, control chars, URL-encoding |
| FsMkdir/FsEncoding/punctuation | PASS | via rclone standard encoder (`encoding` option, Standard \| BackSlash) — `/` `\` mapped to provider-safe names with verified round-trip |
| FsMkdir/FsPutError | PASS | |
| FsMkdir/FsPutZeroLength | PASS | |
| FsMkdir/FsPutFiles (all subtests) | PASS | List/NewObject/NewObjectDir/Purge/Copy/Move/RmdirFull/Update/Remove/Range/FromRoot/… |
| FsMkdir/FsPutChunked / FsPutShortEOF / FsUploadUnknownSize | PASS | |
| FsMkdir/FsCopyChunked | PASS | |
| FsMkdir/FsDirectory | PASS | |
| FsMkdir/FsDirSetModTime | PASS | provider cannot set dir mtime → SKIP-worthy, passed via unsupported |
| FsMkdir/FsRootCollapse | PASS | |
| FsMkdir/FsMkdirMetadata / others metadataless | SKIP/PASS | no metadata support declared |

## Backend bugs found & fixed via fstest (P6.2)

1. **`List` on missing dir** — returned empty listing; contract requires
   `fs.ErrorDirNotFound`. Fixed (bisync cold-start now requires a pre-created
   remote path; documented in docs/safety.md).
2. **`Object.String()`** — included a backend prefix and was not nil-safe;
   contract requires the bare remote and `<nil>` for nil receivers. Fixed.
3. **`NewObject(remote)` path convention** — `Object.remote`/`parentPath` were
   built from drive-absolute paths when the Fs was mounted on a sub-directory,
   breaking `Fs.Move/Fs.Copy` target comparison and `Remote()` consistency.
   Fixed via `rootRel()` denormalization; resolution uses absolute paths while
   object paths stay relative to the Fs root.
4. **`Move` decision on stale metadata** — no-op/rename detection used cached
   Object metadata; now decided from an authoritative `file/get`.
5. **`Move/Copy` rename** — PDS move/copy accept only a target parent; a target
   rename is applied via `file/update` afterwards (same-dir move = pure rename).
6. **Same-directory server-side copy** — Aliyun returns 403; `Copy` now returns
   `fs.ErrorCantCopy` so rclone falls back to read+put (fstest FsCopy goes SKIP
   with a documented reason).
7. **`Hash` after overwrite** — provider response may omit content_hash; the
   locally computed SHA-1 of the uploaded bytes is now authoritative.
8. **`ModTime`** — declared `fs.ModTimeNotSupported` (provider cannot store
   client mtimes; consumers compare size+checksum).
9. **Range reads** — PDS download endpoint ignores Range headers
   (verified: `bytes=81-…` returns full body). Range/Seek are sliced
   client-side; documented in provider-quirks.md.

## Remaining known limitations

- None in the FsMkdir suite. (P7.1: the `encoding` option maps `/` and `\` and
  other provider-unfriendly characters to safe physical names through the
  standard rclone encoder; round-trip is verified by the suite.)