.PHONY: all build build-web build-go dev-web dev-go clean test

# Detect fnm if present
FNM_CMD := $(shell which fnm 2>/dev/null)
ifeq ($(FNM_CMD),)
  NPM_EXEC := npm
else
  NPM_EXEC := eval "$$(fnm env)" && npm
endif

all: build

# Build Vue 3 frontend into internal/web/dist
build-web:
	@echo "=== Building Vue 3 frontend ==="
	cd web && $(NPM_EXEC) install && $(NPM_EXEC) run build

# Build Go single binary with embedded frontend
build-go:
	@echo "=== Building Go binary ==="
	go build -ldflags="-s -w" -o gitina ./cmd/gitina

# One-step full build
build: build-web build-go

# Development Vite dev server
dev-web:
	cd web && $(NPM_EXEC) run dev

# Development Go server
dev-go:
	go run ./cmd/gitina serve -c config.yaml

# Run all unit tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -f gitina
	rm -rf internal/web/dist/assets internal/web/dist/index.html internal/web/dist/logo.svg
