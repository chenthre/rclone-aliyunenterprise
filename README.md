# rclone-aliyunenterprise

> [!IMPORTANT]
> **This repository is vibe-coded.** It has been developed predominantly with
> AI assistance and is released primarily as a *proof of concept / utility*
> rather than a production, peer-reviewed open-source project. It does pass an
> automated gate — unit + race tests, the official rclone `fstest` contract
> suite, and a real-provider command-level gate (`tools/gate-runner.sh`, 10/10)
> — but it has **not** been through full manual code review. Please audit the
> code and test with non-critical data before relying on it for anything
> important, and report issues with a sanitized `-vv` log.

**rclone backend for Aliyun Drive Enterprise (PDS), accessed through the enterprise API Key.**

> **Status: Beta (v0.1.0-rc2)** · Pinned rclone v1.75.1 · Community-maintained,
> not affiliated with Alibaba Cloud.

This repository provides an out-of-tree [rclone] backend (`aliyunenterprise`) that wraps the
Aliyun Drive Enterprise REST data plane into the standard rclone `fs.Fs`/`Object` contract,
so that any rclone user (CLI, scripts, bisync, librclone, future apps) can use an
Aliyun Drive Enterprise space as a normal remote.

```text
any rclone consumer (CLI / bisync / librclone / app)
            │
            ▼
          rclone
            │
    fs.Fs / Object contract
            │
            ▼
   aliyunenterprise backend (this repo)
            │
            ▼
  Aliyun Drive Enterprise REST (api_key)
```

> This is a **rclone backend**, not an app. It does not implement sync algorithms,
> conflict resolution, or any application facade. Those belong to rclone and to
> whichever consumer uses this remote.

---

## 1. Support matrix

| Item | State |
|---|---|
| Service | 阿里云盘企业版 (Aliyun Drive Enterprise/EDM) — **not** PDS Developer Edition |
| Auth | Enterprise API Key `uk-...` as `Authorization: Bearer` |
| Data plane | `https://<domain_id>.api.aliyunfile.com/v2/*` |
| Requires | `domain_id`, `drive_id`, `api_key` (external config, no hardcoding) |
| Other environments | RAM AK / OAuth / `*.api.aliyunpds.com` are **not** assumed |

### Capabilities (verified against a real Aliyun Drive Enterprise domain, 2026-09)

- Root list, create file/folder, upload (multipart + instant/sha1), download,
  get metadata, search, overwrite, rename, move, copy;
- stable `file_id`, SHA-1 `content_hash`;
- logical delete = move to a provider-private hidden trash (no real DELETE available).

### Provider limitations (handled inside the backend)

1. **Sub-directory `file/list` (and `list_delta`) return empty** for this domain →
   backend falls back to `file/search(parent_file_id=<id>, recursive=false)` +
   per-item `file/get` verification (see [implementation notes](docs/implementation-notes.md)).
2. **Search is eventual-consistent** → the backend guarantees *"search absence ≠ deletion"*
   via a persistent safety catalog (see [docs/implementation-notes.md](docs/implementation-notes.md)).
3. **No real delete** → `Remove()` moves to private trash named `_aliyunenterprise_rclone_trash`
   (hidden from listings; manual GC is a non-goal).
4. **No SetModTime** → declared honestly (`fs.ErrorCantSetModTime`); use `--compare size,checksum`
   (bisync) or `--no-update-modtime` (copy).
5. **Unknown/inconsistent states fail closed** — the backend returns an error rather than a
   possibly-incomplete listing that could be misread as deletions.

## 2. Build

Requires Go ≥ 1.22 (module pins `github.com/rclone/rclone v1.75.1`).

```bash
cd rclone-aliyunenterprise
go build -o rclone-aliyunenterprise ./cmd/rclone-aliyunenterprise
./rclone-aliyunenterprise version
./rclone-aliyunenterprise backend list | grep -i aliyun
```

No rclone source modifications; the binary is a normal rclone embedding this backend.

## 3. Configure

Use rclone config, or environment variables (the backend reads
`ALIYUN_ENTERPRISE_API_KEY`, `ALIYUN_ENTERPRISE_DOMAIN_ID`, `ALIYUN_ENTERPRISE_DRIVE_ID`
when config keys are absent):

```bash
export ALIYUN_ENTERPRISE_API_KEY='uk-...'
export ALIYUN_ENTERPRISE_DOMAIN_ID="$(grep '^DOMAIN_ID=' .env | cut -d= -f2)"
export ALIYUN_ENTERPRISE_DRIVE_ID='101'

./rclone-aliyunenterprise lsf :aliyunenterprise: -R
./rclone-aliyunenterprise copy ./vault :aliyunenterprise:backup --checksum
./rclone-aliyunenterprise bisync ./vault :aliyunenterprise:shared \
  --compare size,checksum --create-empty-src-dirs --resilient --recover \
  --max-delete 20 --conflict-resolve none --conflict-loser num --workdir /tmp/bisync-work
```

**Secrets safety**: the API key is never printed, logged, committed, or embedded in the
binary. `.env` and any catalog files are git-ignored. Logs redact authorization headers.

## 4. Backend options

| Option | Env var | Description |
|---|---|---|
| `api_key` | `ALIYUN_ENTERPRISE_API_KEY` | enterprise API key (`uk-...`) |
| `domain_id` | `ALIYUN_ENTERPRISE_DOMAIN_ID` | enterprise domain id (put the real value in `.env`) |
| `drive_id` | `ALIYUN_ENTERPRISE_DRIVE_ID` | drive id inside the domain |
| `hidden_trash_name` | — | private trash folder name (default `_aliyunenterprise_rclone_trash`) |
| `catalog_path` | — | local safety catalog path (default `~/.cache/rclone-aliyunenterprise/catalog.json`) |
| `search_retries` / `search_retry_delay` | — | search fallback retry budget before failing closed |

## 5. Features

Implemented & declared:

- `Fs`: List, NewObject, Put, Mkdir, Rmdir, Purge (logical), Hashes(SHA-1)
- `Object`: Open, Update, Remove, Size, ModTime, Hash(SHA-1), ID
- Optional: server-side `Copy`, server-side `Move`

Honestly *not* declared (rclone falls back / refuses):

- `ListR` (search final-consistency is not a reliable recursive-listing contract)
- `SetModTime` / dir modtime
- `ChangeNotify`, `PublicLink`, `About`, metadata extensions, real delete

## 6. Testing

- Unit tests: `go test ./...` (catalog, errors, utilities, listing semantics).
- Integration: official rclone `fstest` suite — see [docs/fstest-results.md](docs/fstest-results.md)
  and `-tags integration` tests.
- Bisync PoC matrix: see [docs/bisync-poc-results.md](docs/bisync-poc-results.md).

## 7. Documentation

- [Configuration](docs/configuration.md)
- [Provider quirks](docs/provider-quirks.md)
- [Safety model](docs/safety.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Current status / remaining work for v0.1.0](docs/fstest-results.md)

## 9. Known upstream rclone issues

- `bisync` conflict rename bug (`missing info for "...conflict2"`) — see
  [docs/upstream-issues.md](docs/upstream-issues.md).
- `rclone backend list` reports "internal error: no overview data found" for
  this backend (rclone embeds backend docs at build time; out-of-tree backends
  cannot supply one). Registered and fully functional otherwise; see
  [docs/troubleshooting.md](docs/troubleshooting.md).

## 10. Repository layout

```text
rclone-aliyunenterprise/          # Go module: the backend
  ├── backend/aliyunenterprise/   #   implementation
  └── cmd/rclone-aliyunenterprise/#   embedding main (builds the rclone binary)
docs/                            # provider quirks, safety, troubleshooting, results
tools/                           # gate-runner.sh, release.sh
tools/legacy/                    # archived PDS REST probes (reference only)
testdata/                        # bisync fixture data
.github/                         # CI + issue/PR templates
```

## Quick start (new user)

```bash
git clone git@github.com:chenthre/rclone-aliyunenterprise.git
cd rclone-aliyunenterprise/rclone-aliyunenterprise
go build -o rclone-aliyunenterprise ./cmd/rclone-aliyunenterprise

source <(grep -E '^(API_KEY|DOMAIN_ID|DRIVE_ID)=' .env)   # real values live in .env
export ALIYUN_ENTERPRISE_API_KEY="$API_KEY"                 # least-privilege key
export ALIYUN_ENTERPRISE_DOMAIN_ID="$DOMAIN_ID"
export ALIYUN_ENTERPRISE_DRIVE_ID="$DRIVE_ID"'

./rclone-aliyunenterprise lsf :aliyunenterprise: -R            # list
./rclone-aliyunenterprise copy ~/docs :aliyunenterprise:backup # upload
./rclone-aliyunenterprise bisync ~/vault :aliyunenterprise:vault \
  --compare size,checksum --create-empty-src-dirs --resilient --recover \
  --max-delete 20 --conflict-resolve none --conflict-loser num \
  --workdir /tmp/bisync-work --resync        # first run only: mkdir the remote first
```

See [docs/configuration.md](docs/configuration.md), [docs/safety.md](docs/safety.md)
and [docs/troubleshooting.md](docs/troubleshooting.md) for details.

## 11. Non-goals

Any specific app; Android/iOS UI or SAF; app SyncService; sync engine algorithms;
remote distributed locking; custom conflict resolution; real delete/GC; E2EE;
block-level delta; rclone fork; fixing rclone upstream bugs.

[rclone]: https://rclone.org/
