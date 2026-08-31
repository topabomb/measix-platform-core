.PHONY: ci generate generated-drift fmt-check backend-test system-test s01-candidate-test console-test console-e2e e2e-harness contract migrations migration-replay freeze-manifest freeze-validate clean-replay collect-artifacts collect-static-contract freeze-gate collect-baseline collect-adapter-qualification

.NOTPARALLEL:

ci: generate fmt-check contract backend-test system-test console-test migrations generated-drift

fmt-check:
	node scripts/checks.mjs fmt

contract:
	cd backend && go test ./internal/contract -count=1

backend-test:
	cd backend && go test ./... -count=1
	cd backend && go vet ./...
	cd backend && go test -race ./pkg/platformid ./internal/common/health ./internal/common/sqliteutil ./internal/relay/... -count=1
	@# Verify candidate-tagged system scenarios compile, but do NOT run them in backend-test.
	@# They require real Hub/Relay processes and are executed via s01-candidate-test.
	cd backend && go test -tags=candidate -run=^$$ ./test/system/scenarios/ -count=1

# system-test is the bounded T3 lane for default CI. It only runs the
# smoke tests (build tag 'smoke'), NOT the full S0.1 candidate gate.
# It does NOT execute the full CAP-C6 recovery/security/golden-path scenarios.
system-test:
	cd backend && go test -tags=smoke ./test/system/scenarios/ -count=1 -timeout 5m
	cd backend && go test ./test/system/adapter/ ./test/system/client/ -count=1 -timeout 2m

# s01-candidate-test is the explicit S0.1 candidate verification lane.
# It runs the full CAP-C6/C7 deterministic system scenarios including
# recovery, security, and generation tests (build tag 'candidate').
# This target is not part of default GitHub Actions CI/CD; it must be
# executed explicitly on the exact candidate SHA before C6/C7/Freeze claims.
s01-candidate-test: console-build
	cd backend && go test -tags=candidate ./test/system/scenarios/ -count=1 -timeout 15m -v

# e2e-harness runs the full T4.1 clean-environment browser gate:
# temp DB, Hub, Relay, deterministic Adapter, production SPA, Playwright.
# This is the one-click harness required by architecture for S0.1 Gate.
# It uses the Playwright config reporters (list + JSON) without override.
s01-browser-candidate: console-build
	node scripts/e2e-harness.mjs

# Backwards-compatible aliases for s01-browser-candidate
console-e2e: s01-browser-candidate
e2e-harness: s01-browser-candidate

console-build:
	cd console && corepack enable && pnpm install --frozen-lockfile && pnpm build

freeze-manifest:
	node scripts/freeze-manifest.mjs

# freeze-validate validates an existing manifest against current SHA/builds
freeze-validate:
	node scripts/freeze-manifest.mjs --validate

# Full clean-source replay is not implemented. This target fails closed;
# runtime-only diagnostics cannot finalize CAP-C7-002.
clean-replay:
	node scripts/replay-freeze.mjs

# collect-baseline runs the baseline test and generates .artifacts/resource-baseline.json
# Measures architecture §17 required metrics: RSS/CPU, first-byte overhead,
# stream memory, multipart memory/disk, cancel cleanup, spool drain, SQLite growth.
# GREEN is computed from metric completeness, not hardcoded.
collect-baseline:
	node scripts/collect-baseline.mjs

# collect-adapter-qualification generates .artifacts/real-adapter-qualification.json
# Usage: make collect-adapter-qualification ENDPOINT=https://api.openai.com KEY=sk-...
# Implements real qualification runner: Hub → Relay → real Adapter for
# Model streaming, TTS, ASR, MCP profiles per architecture qualification spec.
collect-adapter-qualification:
	node scripts/collect-adapter-qualification.mjs $(if $(ENDPOINT),--endpoint $(ENDPOINT)) $(if $(KEY),--key $(KEY))

# collect-artifacts runs all test suites and writes machine-readable JSON
# results to .artifacts/. These artifacts are consumed by freeze-manifest
# to compile the evidence matrix. Each artifact records the commit SHA
# so the freeze-manifest compiler can verify SHA consistency.
collect-artifacts:
	@mkdir -p .artifacts
	@echo "Collecting backend test artifacts..."
	@(cd backend && go test ./internal/... -count=1 -json -timeout 120s > ../.artifacts/backend-test.json); exit_code=$$?; \
	node scripts/write-meta.mjs backend-test.json 'go test -json' $$exit_code; \
	if [ $$exit_code -ne 0 ]; then echo "ERROR: backend tests failed (exit $$exit_code)"; exit $$exit_code; fi
	@echo "Collecting system test artifacts..."
	@(cd backend && go test -tags=smoke ./test/system/scenarios/ ./test/system/adapter/ ./test/system/client/ -count=1 -json -timeout 5m > ../.artifacts/system-test.json); exit_code=$$?; \
	node scripts/write-meta.mjs system-test.json 'go test -tags=smoke -json' $$exit_code; \
	if [ $$exit_code -ne 0 ]; then echo "ERROR: system tests failed (exit $$exit_code)"; exit $$exit_code; fi
	@echo "Collecting console test artifacts..."
	@(cd console && pnpm vitest run --reporter=json --outputFile=../.artifacts/console-test.json 2>../.artifacts/console-test.stderr.log); exit_code=$$?; \
	node scripts/write-meta.mjs console-test.json 'pnpm vitest run --reporter=json --outputFile' $$exit_code; \
	if [ $$exit_code -ne 0 ]; then echo "ERROR: console tests failed (exit $$exit_code)"; exit $$exit_code; fi
# collect-static-contract runs actual commands and records their exit codes.
# Comments explaining static-contract behavior:
#   - Each check captures exit code AND SHA-256 of command output
#   - PASS/FAIL status is DERIVED from exit code — never hardcoded
#   - Output hash provides tamper-evidence for audit verification
collect-static-contract:
	node scripts/checks.mjs static

# Legacy S0.1 evidence collection entry. NOT a completed C7 gate:
# the compiler rejects current Snapshot v2 and full clean-source replay is
# unimplemented. Preserve historical manifests; do not call this S0.2 Freeze.
# Real adapter qualification must be collected separately in one four-profile run.
freeze-gate: collect-artifacts collect-static-contract s01-candidate-test s01-browser-candidate collect-baseline
	@echo "Collecting candidate test artifacts..."
	@(cd backend && go test -tags=candidate ./test/system/scenarios/ -count=1 -json -timeout 15m > ../.artifacts/candidate-test.json); exit_code=$$?; \
	node scripts/write-meta.mjs candidate-test.json 'go test -tags=candidate -json' $$exit_code; \
	if [ $$exit_code -ne 0 ]; then echo "ERROR: candidate tests failed (exit $$exit_code)"; exit $$exit_code; fi
	@echo "Generating freeze manifest..."
	node scripts/freeze-manifest.mjs
	@echo "Running clean replay verification (CAP-C7-002)..."
	node scripts/replay-freeze.mjs

console-test:
	cd console && corepack enable && pnpm install --frozen-lockfile && pnpm typecheck && pnpm test && pnpm build

migrations: migration-replay
	cd backend && go run ./cmd/migration-checksum
	git diff --exit-code -- backend/migrations/atlas.sum

migration-replay:
	@tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	atlas migrate apply --dir file://backend/migrations --url "sqlite://$tmp/hub.db" && \
	atlas migrate status --dir file://backend/migrations --url "sqlite://$$tmp/hub.db"

generated-drift:
	node scripts/checks.mjs drift

generate:
	node scripts/checks.mjs generate
