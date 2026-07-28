# FeatureForge developer commands.
#
# `make verify` is the full suite. `make test` alone is meaningful without
# Docker: the PostgreSQL-backed tests skip cleanly when
# FEATUREFORGE_POSTGRES_TEST_DSN is unset, rather than being hidden behind a
# build tag that would silently not compile.

# Override FEATUREFORGE_POSTGRES_PORT if 55433 is taken on your machine; both
# the container's published port and the DSN follow it.
FEATUREFORGE_POSTGRES_PORT ?= 55433
export FEATUREFORGE_POSTGRES_PORT
POSTGRES_TEST_DSN ?= postgres://featureforge:featureforge@localhost:$(FEATUREFORGE_POSTGRES_PORT)/featureforge?sslmode=disable
COMPOSE ?= docker compose -f docker-compose.test.yml

.PHONY: fmt vet build test race postgres-up postgres-down postgres-test verify

fmt:
	gofmt -l .

vet:
	go vet ./...

build:
	go build ./...

# Runs everything that does not need a database; PostgreSQL tests skip.
test:
	go test ./... -count=1

race:
	go test ./... -race -count=1

postgres-up:
	$(COMPOSE) up -d --wait

postgres-down:
	$(COMPOSE) down -v

# Starts PostgreSQL, applies migrations (the test harness does this itself),
# runs every PostgreSQL-backed test, and tears the database down again.
postgres-test: postgres-up
	FEATUREFORGE_POSTGRES_TEST_DSN="$(POSTGRES_TEST_DSN)" \
		go test ./internal/infrastructure/postgres/... ./internal/scenario/... ./internal/transport/http/... -count=1 -v; \
		status=$$?; $(COMPOSE) down -v; exit $$status

# The single command that verifies the whole milestone.
verify: fmt vet build test race postgres-test
