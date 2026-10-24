# Development targets for the AI development platform (Linux / WSL).
# build and run delegate to the cross-platform cmd/devctl tool; the rest
# stays Unix-specific.
BINARY := bin/platform

.PHONY: help build run test vet web-install web-build web-test web-typecheck web-lint e2e web-legacy-install web-legacy-build web-legacy-test web-legacy-typecheck cutover-react rollback-react verify release clean

help: ## Show the available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

build: ## Build the platform server binary into bin/platform
	go run ./cmd/devctl build

run: ## Run the platform server from source (same-origin UI from web/dist)
	go run ./cmd/devctl run

test: ## Run the Go test suite
	go test ./...

vet: ## Run go vet over all packages
	go vet ./...

web-install: ## Install React frontend dependencies (web-next/)
	cd web-next && npm ci

web-build: ## Build React and copy into web/dist (served by the Go service)
	cd web-next && npm run build
	rm -rf web/dist
	cp -R web-next/dist web/dist

web-test: ## Run the React frontend unit tests once
	cd web-next && npm run test:run

web-typecheck: ## Type-check the React frontend
	cd web-next && npm run typecheck

web-lint: ## Lint the React frontend
	cd web-next && npm run lint

e2e: ## Run the React Playwright E2E suite against the real stack
	cd web-next && npm run test:e2e

web-legacy-install: ## Install the rollback Vue dependencies (web/)
	cd web && npm ci

web-legacy-build: ## Rebuild and serve the rollback Vue app
	cd web && npm run build

web-legacy-test: ## Run the rollback Vue unit tests
	cd web && npm test -- --run

web-legacy-typecheck: ## Type-check the rollback Vue app
	cd web && npm run typecheck

cutover-react: ## Back up web/dist and switch production to the React build
	./scripts/cutover-react.sh

rollback-react: ## Restore the Vue build captured by cutover-react
	./scripts/rollback-react.sh

verify: vet test web-test web-typecheck web-lint web-build ## Run everything: vet, Go tests, React tests/typecheck/lint/build

release: ## Build Linux/Windows packages into release/ (see ./release.sh help)
	./release.sh

clean: ## Remove build artifacts
	rm -rf bin
