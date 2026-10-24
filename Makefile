SHELL := /bin/sh
VERSION ?= 0.1.0
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo local)
GO ?= go
PNPM ?= pnpm
IMAGE ?= niridesk:$(VERSION)
GOENV := GOCACHE="$(CURDIR)/.cache/go-build" GOMODCACHE="$(CURDIR)/.cache/go/pkg/mod" GOPATH="$(CURDIR)/.cache/go" GOFLAGS=-mod=mod

.PHONY: help init web-deps web-check web-build dev-web build test vet fmt check run docker-build up down logs clean

help:
	@printf '%s\n' \
		'make init          Generate .env with secrets and detected Wayland display' \
		'make web-deps      Install frontend dependencies with pnpm' \
		'make web-build     Type-check and build the Solid frontend into web/dist' \
		'make dev-web       Start the Vite dev server (proxy to :8080)' \
		'make check         Run Go checks, frontend type-check, and tests' \
		'make build         Build frontend and local binary into bin/niridesk' \
		'make docker-build  Build the container image' \
		'make up            Build and start with Docker Compose' \
		'make down          Stop the Docker Compose stack' \
		'make logs          Follow container logs' \
		'make clean         Remove local build, frontend, and cache output'

init:
	./scripts/init-env.sh

web-deps:
	cd web && $(PNPM) install --frozen-lockfile

web-check: web-deps
	cd web && $(PNPM) check

web-build: web-deps
	cd web && $(PNPM) build

dev-web: web-deps
	cd web && $(PNPM) dev

build: web-build
	@mkdir -p bin
	$(GOENV) $(GO) build -trimpath -ldflags="-X main.version=$(VERSION) -X main.commit=$(COMMIT)" -o bin/niridesk ./cmd/niridesk

test:
	$(GOENV) $(GO) test ./...

vet:
	$(GOENV) $(GO) vet ./...

fmt:
	$(GOENV) $(GO) fmt ./...

check: web-check
	@test -z "$$(gofmt -l cmd internal web/embed.go)" || (echo "gofmt needed: $$(gofmt -l cmd internal web/embed.go)"; exit 1)
	$(GOENV) $(GO) vet ./...
	$(GOENV) $(GO) test ./...

run: web-build
	$(GOENV) $(GO) run ./cmd/niridesk

docker-build:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t $(IMAGE) .

up:
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f --tail=100

clean:
	chmod -R u+w .cache 2>/dev/null || true
	rm -rf bin .cache web/node_modules web/dist
	mkdir -p web/dist
	touch web/dist/.gitkeep
