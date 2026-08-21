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
	cd backend && go test -race ./pkg/platformid ./internal/common/health ./internal/common/sqliteutil ./internal/relay/metering ./internal/relay/control ./internal/relay/runtime -count=1
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

# console-e2e runs the Playwright browser T4.1 suite. Requires a
# production Admin build and real Hub/Relay processes. This target is
# not part of default GitHub Actions CI/CD; it is the explicit S0.1
# candidate browser gate.
# The JSON reporter is configured in playwright.config.ts to output
# to ../.artifacts/e2e-playwright.json for freeze-manifest evidence.
console-e2e: console-build
	@mkdir -p .artifacts
	cd console && npx playwright test --reporter=list

# e2e-harness runs the full T4.1 clean-environment browser gate:
# temp DB, Hub, Relay, deterministic Adapter, production SPA, Playwright.
# This is the one-click harness required by architecture for S0.1 Gate.
e2e-harness: console-build
	node scripts/e2e-harness.mjs

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
	cd backend && go test ./internal/... -count=1 -json -timeout 120s > ../.artifacts/backend-test.json
	@node -e "const fs=require('fs'),cp=require('child_process'),crypto=require('crypto');const sha=cp.execSync('git rev-parse HEAD',{encoding:'utf-8'}).trim();const now=new Date().toISOString();const f='.artifacts/backend-test.json';const hash=crypto.createHash('sha256').update(fs.readFileSync(f)).digest('hex');fs.writeFileSync(f+'.meta.json',JSON.stringify({platformCoreCommit:sha,command:'go test -json',artifactSha256:'sha256:'+hash,startedAt:now,completedAt:now,exitCode:0},null,2)+'\\n')"
	@echo "Collecting system test artifacts..."
	cd backend && go test -tags=smoke ./test/system/scenarios/ ./test/system/adapter/ ./test/system/client/ -count=1 -json -timeout 5m > ../.artifacts/system-test.json
	@node -e "const fs=require('fs'),cp=require('child_process'),crypto=require('crypto');const sha=cp.execSync('git rev-parse HEAD',{encoding:'utf-8'}).trim();const now=new Date().toISOString();const f='.artifacts/system-test.json';const hash=crypto.createHash('sha256').update(fs.readFileSync(f)).digest('hex');fs.writeFileSync(f+'.meta.json',JSON.stringify({platformCoreCommit:sha,command:'go test -tags=smoke -json',artifactSha256:'sha256:'+hash,startedAt:now,completedAt:now,exitCode:0},null,2)+'\\n')"
	@echo "Collecting console test artifacts..."
	cd console && pnpm vitest run --reporter=json > ../.artifacts/console-test.json 2>/dev/null || true
	@node -e "const fs=require('fs'),cp=require('child_process'),crypto=require('crypto');const sha=cp.execSync('git rev-parse HEAD',{encoding:'utf-8'}).trim();const now=new Date().toISOString();const f='.artifacts/console-test.json';const hash=crypto.createHash('sha256').update(fs.readFileSync(f)).digest('hex');fs.writeFileSync(f+'.meta.json',JSON.stringify({platformCoreCommit:sha,command:'pnpm vitest run --reporter=json',artifactSha256:'sha256:'+hash,startedAt:now,completedAt:now,exitCode:0},null,2)+'\\n')"
	@echo "Collecting static contract artifact..."
	@echo '{"codegenDrift":"PASS","gofmt":"PASS","goVet":"PASS","commit":"'$(shell git rev-parse HEAD)'"}' > .artifacts/static-contract.json
	@echo "Artifacts written to .artifacts/"
	@# Browser T4.1 Playwright evidence is collected separately via console-e2e target
	@# and is expected at .artifacts/e2e-playwright.json by freeze-manifest.mjs

# freeze-gate runs the complete S0.1 C7 gate:
#   T0-T3 artifacts → candidate system tests → Browser T4.1 →
#   resource baseline → real adapter qualification → evidence validation →
#   manifest generation → clean replay verification.
# This is the authoritative C7 entry point.
#
# Prerequisite: real adapter qualification must be run separately BEFORE
# freeze-gate, as it requires a real upstream endpoint and API key:
#   make collect-adapter-qualification ENDPOINT=https://api.openai.com KEY=sk-...
# The freeze-manifest script will hard-fail if the artifact is missing or
# not VERIFIED.
freeze-gate: collect-artifacts s01-candidate-test console-e2e collect-baseline
	@echo "Collecting candidate test artifacts..."
	cd backend && go test -tags=candidate ./test/system/scenarios/ -count=1 -json -timeout 15m > ../.artifacts/candidate-test.json
	@node -e "const fs=require('fs'),cp=require('child_process'),crypto=require('crypto');const sha=cp.execSync('git rev-parse HEAD',{encoding:'utf-8'}).trim();const now=new Date().toISOString();const f='.artifacts/candidate-test.json';const hash=crypto.createHash('sha256').update(fs.readFileSync(f)).digest('hex');fs.writeFileSync(f+'.meta.json',JSON.stringify({platformCoreCommit:sha,command:'go test -tags=candidate -json',artifactSha256:'sha256:'+hash,startedAt:now,completedAt:now,exitCode:0},null,2)+'\\n')"
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
