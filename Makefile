.PHONY: ci generate generated-drift fmt-check backend-test system-test console-test contract migrations migration-replay freeze-manifest

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

system-test:
	cd backend && go test ./test/system/... -count=1 -timeout 5m

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
