# AI-assisted changelog

## 2026-08-10 - New-CMS architecture bootstrap and inherited HAL audit

- Reframed this repository as the independently evolving OCPP HAL for `ev-cms-backend-new`, separate from the legacy `OCPPHAL_Go` and old-CMS integration.
- Replaced inherited project rules, overview, development plan, project state, and documentation navigation that treated legacy CMS/frontend compatibility as this repository's purpose.
- Added the canonical HAL/CMS architecture boundary record and an evidence-based inherited subsystem audit, including preservation properties for transaction truth, stale-disconnect protection, callbacks, retries, recovery, metering, and local tooling.
- Recorded that no new CMS/HAL endpoint, event, callback, table schema, or authentication design is approved yet, and sequenced a joint audit with `ev-cms-backend-new` wallet, tariff, and charging-session requirements.
- Recorded the inherited legacy-root paths in build/regression automation as a bootstrap defect so future work does not accidentally operate on the legacy repository.

Compatibility: no Go runtime behavior, route, database schema, callback payload, deployment, or external service was changed. The legacy repository and `ev-cms-backend-new` were not modified.

Verification: source paths and representative OCPP, persistence, recovery, callback, API, configuration, migration, and tooling call sites were inspected. `go test ./...` was terminated after 124 seconds with no diagnostic output. `go build ./...` reached `cmd/cpconsole` and failed because the Go linker could not allocate memory. Inherited scripts were intentionally not run because their absolute legacy path would operate outside this checkout. The final documentation diff and `git diff --check` were also required for this slice.

## 2026-08-03 — Terminal-controlled virtual charger

- Added `cmd/cpconsole`, a standalone configurable OCPP 1.6J virtual EV charger.
- Added coherent terminal-controlled local/remote transactions and meter progression.
- Added fault, suspension, remote-policy, diagnostics, firmware and trigger behavior.
- Added Windows build registration and Linux amd64/arm64 cross-build tooling.
- Added focused parser tests and the complete operator/integration guide.
- Registered canonical project-memory files and recorded existing OpenAPI remediation.

Compatibility: no HAL server route, database schema, callback payload, or runtime
ownership boundary changed. The new program is an additional test client.

Verification: focused tests, `go test ./...`, all Windows builds, Linux amd64
cross-build, and a loopback memory-store charge flow passed. The canonical
PostgreSQL regression could not run non-interactively because its password prompt
received no value; this entry does not claim that regression passed.
