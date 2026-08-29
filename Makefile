.PHONY: test lint fmt run migrate compose-up compose-down tidy test-python

GO ?= go
# Default proxy helps environments where proxy.golang.org is unreachable.
export GOPROXY ?= https://goproxy.cn,direct
PYTHON ?= python3

test: test-go test-python

test-go:
	$(GO) test ./...

test-python:
	cd sdk/python && PYTHONPATH=. $(PYTHON) -m unittest discover -s tests -v

lint:
	$(GO) vet ./...
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:" && gofmt -l . && exit 1)

fmt:
	gofmt -w .

tidy:
	$(GO) mod tidy

run:
	$(GO) run ./cmd/server -config configs/config.yaml

migrate:
	$(GO) run ./cmd/server -config configs/config.yaml -migrate-only

compose-up:
	docker compose -f deployments/docker-compose.yml up -d --build

compose-down:
	docker compose -f deployments/docker-compose.yml down -v

compose-postgres:
	docker compose -f deployments/docker-compose.yml up -d postgres
