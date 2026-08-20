.PHONY: ci generate generated-drift fmt-check backend-test system-test s01-candidate-test console-test console-e2e e2e-harness contract migrations migration-replay freeze-manifest

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
	cd backend && go test -race ./pkg/platformid ./internal/common/health ./internal/common/sqliteutil ./internal/relay/metering -count=1
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
console-e2e: console-build
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
