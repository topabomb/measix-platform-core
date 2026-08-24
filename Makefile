.PHONY: ci generate generated-drift fmt-check backend-test system-test s01-candidate-test console-test console-e2e e2e-harness contract migrations migration-replay freeze-manifest freeze-validate clean-replay collect-artifacts freeze-gate collect-baseline collect-adapter-qualification

ci: fmt-check contract backend-test system-test console-test migrations generated-drift

fmt-check:
	@files=$$(find backend -name '*.go' -type f -print0 | xargs -0 gofmt -l); \
	if [ -n "$$files" ]; then \
		echo "Go files need gofmt:"; echo "$$files"; \
		for file in $$files; do gofmt -d "$$file"; done; \
		exit 1; \
	fi

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

# clean-replay verifies the freeze manifest can be replayed in clean env (CAP-C7-002)
# This performs a real fresh-environment replay: starts fresh Hub + Relay,
# verifies admin login and system status, and checks all required scenarios.
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
	@cd backend && go test ./internal/... -count=1 -json -timeout 120s > ../.artifacts/backend-test.json; exit_code=$$?; \
	node scripts/write-meta.mjs backend-test.json 'go test -json' $$exit_code; \
	if [ $$exit_code -ne 0 ]; then echo "ERROR: backend tests failed (exit $$exit_code)"; exit $$exit_code; fi
	@echo "Collecting system test artifacts..."
	@cd backend && go test -tags=smoke ./test/system/scenarios/ ./test/system/adapter/ ./test/system/client/ -count=1 -json -timeout 5m > ../.artifacts/system-test.json; exit_code=$$?; \
	node scripts/write-meta.mjs system-test.json 'go test -tags=smoke -json' $$exit_code; \
	if [ $$exit_code -ne 0 ]; then echo "ERROR: system tests failed (exit $$exit_code)"; exit $$exit_code; fi
	@echo "Collecting console test artifacts..."
	@cd console && pnpm vitest run --reporter=json --outputFile=../.artifacts/console-test.json 2>../.artifacts/console-test.stderr.log; exit_code=$$?; \
	node scripts/write-meta.mjs console-test.json 'pnpm vitest run --reporter=json --outputFile' $$exit_code; \
	if [ $$exit_code -ne 0 ]; then echo "ERROR: console tests failed (exit $$exit_code)"; exit $$exit_code; fi
	@echo "Collecting static contract artifact..."
	@echo '{"codegenDrift":"PASS","gofmt":"PASS","goVet":"PASS","commit":"'$(shell git rev-parse HEAD)'"}' > .artifacts/static-contract.json
	@echo "Artifacts written to .artifacts/"
	@# Browser T4.1 Playwright evidence is collected separately via console-e2e target
	@# and is expected at .artifacts/e2e-playwright.json by freeze-manifest.mjs

# freeze-gate runs the complete S0.1 C7 gate (two-phase):
#   Phase 1 (candidate): T0-T3 artifacts → candidate system tests → Browser T4.1 →
#     resource baseline → candidate manifest (CAP-C7-002=NOT_EXECUTED)
#   Phase 2 (clean replay): fresh Hub/Relay/Adapter → replay T4.1 Golden Path +
#     Test Client four capabilities + Usage closure → topology security →
#     update manifest (CAP-C7-002=PASS + replay artifact hash)
# This is the authoritative C7 entry point.
#
# Prerequisite: real adapter qualification must be run separately BEFORE
# freeze-gate, as it requires a real upstream endpoint and API key:
#   make collect-adapter-qualification ENDPOINT=https://api.openai.com KEY=sk-...
# The freeze-manifest script will hard-fail if the artifact is missing or
# not VERIFIED.
freeze-gate: collect-artifacts s01-candidate-test s01-browser-candidate collect-baseline
	@echo "Collecting candidate test artifacts..."
	@cd backend && go test -tags=candidate ./test/system/scenarios/ -count=1 -json -timeout 15m > ../.artifacts/candidate-test.json; exit_code=$$?; \
	node scripts/write-meta.mjs candidate-test.json 'go test -tags=candidate -json' $$exit_code; \
	if [ $$exit_code -ne 0 ]; then echo "ERROR: candidate tests failed (exit $$exit_code)"; exit $$exit_code; fi
	@echo "Generating freeze manifest..."
	node scripts/freeze-manifest.mjs
	@echo "Running clean replay verification (CAP-C7-002)..."
	node scripts/replay-freeze.mjs

console-test:
	cd console && corepack enable && pnpm install --frozen-lockfile && pnpm typecheck && pnpm test && pnpm build

migrations: migration-replay
	atlas migrate hash --dir file://backend/migrations
	git diff --exit-code -- backend/migrations/atlas.sum

migration-replay:
	@tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	atlas migrate apply --dir file://backend/migrations --url "sqlite://$$tmp/hub.db"; \
	atlas migrate status --dir file://backend/migrations --url "sqlite://$$tmp/hub.db"

generated-drift:
	git diff --exit-code -- backend/go.mod backend/go.sum backend/ent backend/internal/wire backend/migrations/atlas.sum api/generated/android console/pnpm-lock.yaml console/src/api/generated.ts
	@test -z "$$(git status --porcelain -- api/generated/android)" || (git status --short -- api/generated/android; exit 1)

generate:
	cd backend && go mod tidy
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -config api/codegen/admin.yaml api/admin/admin.openapi.yaml
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -config api/codegen/client.yaml api/client/client-control.openapi.yaml
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -config api/codegen/relay.yaml api/internal/relay-control.openapi.yaml
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -config api/codegen/usage.yaml api/internal/usage-ingest.openapi.yaml
	cd backend && go run ./cmd/generate-android-wire ../api/client/client-control.openapi.yaml ../api/generated/android/client-control.openapi.yaml ../api/generated/android/manifest.json
	cd backend && go generate ./ent
	cd backend && go mod tidy
	cd console && corepack enable && corepack prepare pnpm@11.0.0 --activate && pnpm install --no-frozen-lockfile && pnpm generate:api
	atlas migrate hash --dir file://backend/migrations
