# hatchet-aquifer

An **unofficial** fork of [Hatchet](https://github.com/hatchet-dev/hatchet) with a small, opt-in patchset for the [Aqueduct](https://github.com/rjpruitt16/aquifer) backpressure protocol, used by the benchmarks in [aqueduct-benchmarks](https://github.com/rjpruitt16/aqueduct-benchmarks). It is not supported by the Hatchet team. Report Hatchet bugs upstream, and only patch-specific problems here.

Each `aqueduct/<hatchet-version>` branch is that upstream release plus the patches below. Nothing else changes, and upstream licensing and attribution are kept.

## Patches

1. **429 for ResourceExhausted on the REST trigger.** `POST /api/v1/stable/tenants/{tenant}/workflow-runs/trigger` answered the engine's `ResourceExhausted` with 400 Bad Request, so HTTP callers treated backpressure as a malformed request. It now answers 429 Too Many Requests. When the cause is the engine's trigger write slots being full, it also sends `Retry-After: 1`; tenant limits, which reset on a long window, get no `Retry-After`. Files: `api/v1/server/handlers/v1/workflow-runs/aqueduct.go`, one line in `trigger.go`.

## Images

`.github/workflows/aqueduct-images.yml` runs the patch tests and builds `ghcr.io/rjpruitt16/hatchet-aquifer-<target>:<hatchet-version>-aqueduct-<commit>` (linux/amd64) on every push to an `aqueduct/*` branch. Targets other than the patched ones use the upstream images unchanged.
