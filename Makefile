.PHONY: ci generate backend-test console-test contract migrations
ci: contract backend-test console-test migrations
contract:
	cd backend && go test ./internal/contract -count=1
backend-test:
	cd backend && go test ./... -count=1
	cd backend && go vet ./...
	cd backend && go test -race ./pkg/platformid ./internal/common/health -count=1
console-test:
	cd console && corepack enable && pnpm install --frozen-lockfile && pnpm typecheck && pnpm test && pnpm build
migrations:
	atlas migrate hash --dir file://backend/migrations
	git diff --exit-code -- backend/migrations/atlas.sum

generate:
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -config api/codegen/admin.yaml api/admin/admin.openapi.yaml
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -config api/codegen/client.yaml api/client/client-control.openapi.yaml
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -config api/codegen/relay.yaml api/internal/relay-control.openapi.yaml
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -config api/codegen/usage.yaml api/internal/usage-ingest.openapi.yaml
	cd backend && go generate ./ent
	cd backend && go mod tidy
	cd console && corepack enable && corepack prepare pnpm@11.0.0 --activate && pnpm install --no-frozen-lockfile && pnpm generate:api
	atlas migrate hash --dir file://backend/migrations
