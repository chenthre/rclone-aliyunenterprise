# Security Policy

## Reporting a vulnerability

Please **do not open a public issue** for security problems.

Contact the maintainers privately via GitHub (see repository profile) or open
a [draft security advisory](https://github.com/OWNER/rclone-aliyunenterprise/security/advisories/new)
with as much detail as possible:

- affected version (backend + pinned rclone)
- what you were doing
- sanitized debug log (credentials removed)
- minimal reproduction if available

We will acknowledge within 48h and aim for a coordinated fix.

## Scope

This project is a small community backend; the security-sensitive surface is:

- credential handling (never log/commit API keys or signed URLs)
- catalog file integrity (corruption must fail closed)
- logical-delete semantics (never expose provider-private trash)
- fail-closed listing behavior (never report uncertain state as deletion)

## Secrets

- API Keys must never be committed, pasted into issues, or printed in full.
- Prefer the least-privileged API Key for the target drive, with an expiry.
- Rotate immediately after any suspected leak.

## Supported versions

| Version | Supported |
|---|---|
| main | yes (dev) |
| >= v0.1.0 | yes |
| < v0.1.0 | no |

## Reporting notes for third-party issues

- rclone bisync/upstream bugs: report to https://github.com/rclone/rclone/issues
  (minimal reproducer, no credentials).
- Aliyun API behavior changes/permissions: contact Aliyun support; provide a
  sanitized error payload.