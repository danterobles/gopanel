include tools/versions.mk
export GOTOOLCHAIN := go1.26.10
GO ?= go

.PHONY: dev generate assets test test-integration lint fmt build migrate-up migrate-down seed smoke dev-compose generate-sql compose-migrate-up smoke-auth smoke-rbac

dev: build
	sh scripts/with-env.sh ./bin/gopanel

dev-compose:
	docker compose --env-file .env.compose up --build --wait

compose-migrate-up:
	docker compose --env-file .env.compose run --rm --build --entrypoint /migrate app up

assets:
	npm ci --no-audit --no-fund
	npm run build

generate: assets generate-sql
	$(GO) run github.com/a-h/templ/cmd/templ@$(TEMPL_VERSION) generate

generate-sql:
	$(GO) run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

fmt:
	gofmt -w $$(find cmd internal web migrations tests -name '*.go')

lint:
	@test -z "$$(gofmt -l $$(find cmd internal web migrations tests -name '*.go'))" || (echo 'Ejecutar make fmt'; exit 1)
	$(GO) vet ./...

test:
	$(GO) test ./...

test-integration:
	sh scripts/with-env.sh $(GO) test -tags=integration ./... -count=1 -v

build:
	CGO_ENABLED=0 $(GO) build -trimpath -o bin/gopanel ./cmd/server

migrate-up:
	sh scripts/with-env.sh $(GO) run ./cmd/migrate up

migrate-down:
	sh scripts/with-env.sh $(GO) run ./cmd/migrate down

seed:
	sh scripts/with-env.sh $(GO) run ./cmd/seed

# Native smoke never stops the local MySQL server.
smoke:
	sh scripts/with-env.sh python3 scripts/smoke-native.py

smoke-auth: build
	sh scripts/with-env.sh python3 scripts/smoke-auth-native.py

smoke-rbac: build
	sh scripts/with-env.sh python3 scripts/smoke-rbac-native.py

.PHONY: test-e2e bench backup
test-e2e: build
	sh scripts/with-env.sh $(GO) test -tags=e2e ./tests/e2e -count=1 -timeout=3m -v

bench: build
	sh scripts/with-env.sh $(GO) test -tags=load ./tests/e2e -run TestLoadHTML -count=1 -timeout=5m -v

backup:
	sh scripts/with-env.sh $(GO) run ./cmd/backup
