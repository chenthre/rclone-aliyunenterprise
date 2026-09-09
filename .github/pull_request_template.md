## What does this PR do?

<!-- one-liner -->

## Scope check

- [ ] Backend contract / capability / docs / tests
- [ ] If a provider workaround is added/changed: a regression test is included
- [ ] If fstest behavior changed: docs/fstest-results.md updated

## Verification

- [ ] `go fmt`, `go vet ./...`, `go test ./...` pass
- [ ] integration build (`-tags integration`) compiles
- [ ] CHANGELOG entry added (Unreleased)

## Safety

- [ ] No API keys / signed URLs in diff, commits, or logs
- [ ] No behavior that reports uncertain provider state as deletion