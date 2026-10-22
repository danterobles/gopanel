include tools/versions.mk
export GOTOOLCHAIN := go1.26.10
GO ?= go

.PHONY: dev generate assets test test-integration lint fmt build migrate-up migrate-down seed smoke

dev:
	docker compose up --build --wait

assets:
	npm ci --no-audit --no-fund
	npm run build

generate: assets
	$(GO) run github.com/a-h/templ/cmd/templ@$(TEMPL_VERSION) generate

fmt:
	gofmt -w $$(find cmd internal web -name '*.go')

lint:
	@test -z "$$(gofmt -l $$(find cmd internal web -name '*.go'))" || (echo 'Ejecutar make fmt'; exit 1)
	$(GO) vet ./...

test:
	$(GO) test ./...

test-integration:
	$(GO) test -tags=integration ./internal/platform/database -count=1 -v

build:
	CGO_ENABLED=0 $(GO) build -trimpath -o bin/gopanel ./cmd/server

migrate-up migrate-down:
	@echo 'Pendiente etapa 2: aún no existen migraciones; herramienta fijada en tools/versions.mk.' >&2
	@exit 1

seed:
	@echo 'Pendiente etapa 2: aún no existe seed.' >&2
	@exit 1

# Stops/restarts only the MySQL and app services of the current Compose project.
smoke:
	python3 scripts/smoke.py
