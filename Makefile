# Development targets for the AI development platform (Linux / WSL).
BINARY := bin/platform

.PHONY: help build run test vet web-install web-build web-test e2e verify clean

help: ## Show the available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

build: ## Build the platform server binary into bin/platform
	go build -o $(BINARY) ./cmd/platform

run: ## Run the platform server from source (same-origin UI from web/dist)
	go run ./cmd/platform -web-dir web/dist

test: ## Run the Go test suite
	go test ./...

vet: ## Run go vet over all packages
	go vet ./...

web-install: ## Install frontend dependencies (web/)
	cd web && npm ci

web-build: ## Build the frontend into web/dist (served by the Go service)
	cd web && npm run build

web-test: ## Run the frontend unit tests once
	cd web && npm test -- --run

e2e: ## Run the Playwright E2E suite against the real stack
	cd web && npm run test:e2e

verify: vet test web-test web-build ## Run everything: vet, Go tests, frontend tests, frontend build

clean: ## Remove build artifacts
	rm -rf bin
