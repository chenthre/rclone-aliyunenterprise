# RC soak test plan (P7.6)

Purpose: run the release candidate in realistic non-critical use for 3-7 days
and watch for correctness regressions before declaring v0.1.0.

## Setup

- Use **non-critical** data (never a production vault).
- Use the binary from the RC tag (`v0.1.0-rc2`); pin it.
- Keep a fresh catalog per profile (`~/.cache/rclone-aliyunenterprise/`).

## Daily loop (each day)

```bash
# A-side push
rclone-aliyunenterprise bisync ~/soak-a :aliyunenterprise:soak \
  --compare size,checksum --create-empty-src-dirs --resilient --recover \
  --max-delete 20 --conflict-resolve none --conflict-loser num \
  --workdir /tmp/soak-work-a
# B-side pull (simulated second device)
rclone-aliyunenterprise bisync ~/soak-b :aliyunenterprise:soak \
  --compare size,checksum --create-empty-src-dirs --resilient --recover \
  --max-delete 20 --conflict-resolve none --conflict-loser num \
  --workdir /tmp/soak-work-b
```

Rotate scenarios across days:
- new files (unicode, spaces, `/`-`\` names, zero-byte, medium binary)
- modify existing · rename (moveto) · delete (logical → hidden trash)
- run `tools/gate-runner.sh` each day
- one deliberate invalid-key run (must fail closed, 0 destructive ops)
- one killed-process run (lock/recover behavior)

## Watch list

- unexpected deletion or wrong-object operations (blocking)
- catalog errors / ambiguous path / stale-listing loops
- range fallback / retry storms
- trash growth (does not self-GC; expected)
- performance anomalies vs file count

## Decision

All clean after ≥3 days without a correctness bug → `v0.1.0` gate.
Any blocking item (§9 of the v4 guide) → fix, re-tag rc3.
