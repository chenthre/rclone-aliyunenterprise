# Contributing to rclone-aliyunenterprise

Thanks for considering a contribution. This project is an out-of-tree rclone
backend for Aliyun Drive Enterprise. Before opening an issue or PR, please read
the [implementation notes](docs/implementation-notes.md) and
[provider quirks](docs/provider-quirks.md) — many behaviors are deliberate
adjustments to provider limitations.

## Scope

This repository implements **the rclone Backend only**. We do **not** accept:

- rclone sync/bisync algorithm changes
- conflict engine / lock workarounds (upstream rclone)
- any app-specific orchestration, UI, or Android/iOS code
- provider "delete" workarounds beyond the hidden-trash design

## Issues

Please use the issue templates and classify the problem:

| Category | Where it belongs |
|---|---|
| Backend bug (contract violation, wrong listing, data loss risk) | this repo |
| Aliyun API behavior / permission / Search-List semantics change | this repo (provider issue template) |
| Feature request that is a rclone Backend capability | this repo |
| bisync / lock / conflict behavior reproducible with official backends | rclone upstream |
| App integration | a consumer project, not here |

Always include (sanitized):

- rclone version (pinned), backend version/commit, Go version
- exact command line
- `-vv` log with credentials/signed URLs removed

**Never paste API keys or full signed URLs in any issue.**

## Development workflow

```bash
cd rclone-aliyunenterprise
go build -o rclone-aliyunenterprise ./cmd/rclone-aliyunenterprise
go test ./...          # unit (offline)
go vet ./...           # + gofmt -l backend cmd
# live contract tests (require real enterprise credentials):
ALIYUN_ENTERPRISE_API_KEY=... ALIYUN_ENTERPRISE_DOMAIN_ID=... ALIYUN_ENTERPRISE_DRIVE_ID=... \
  go test -tags integration -run 'TestContract/FsMkdir' -timeout 30m -args -verbose ./backend/aliyunenterprise/
```

Every provider-specific workaround **must** come with a regression test
(catalog/lister unit tests, or the fstest contract suite).

## Definition of done

- `go fmt`, `go vet`, `go test ./...` pass
- workaround has regression coverage
- docs updated (`docs/implementation-notes.md`, `docs/provider-quirks.md`,
  `docs/fstest-results.md` as appropriate)
- no secrets in diff, commits, or logs
- CHANGELOG entry (unreleased section)

## Commit style

- Conventional Commits: `feat:`, `fix:`, `test:`, `docs:`, `chore:`
- one logical change per commit
- reference the P-stage or issue where relevant

Thanks for keeping the backend honest and safe.