# Development targets for the AI development platform (Linux / WSL).
# build and run delegate to the cross-platform cmd/devctl tool; the rest
# stays Unix-specific.
BINARY := bin/platform

.PHONY: help build build-win run test vet web-install web-build web-test web-typecheck desktop-install desktop-build desktop-test desktop-package desktop-package-win e2e verify release clean

help: ## Show the available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

build: ## Build the platform server binary into bin/platform
	go run ./cmd/devctl build

build-win: ## Cross-compile the platform server binary for Windows (bin/platform.exe)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/platform.exe ./cmd/platform

run: ## Run the platform server from source (same-origin UI from web/dist)
	go run ./cmd/devctl run

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

web-typecheck: ## Type-check the frontend (tsc; .vue SFCs are not covered)
	cd web && npm run typecheck

desktop-install: ## Install desktop app dependencies (desktop/)
	cd desktop && npm install

desktop-build: ## Build desktop TypeScript source (desktop/)
	cd desktop && npm run build

desktop-test: ## Run desktop unit tests once
	cd desktop && npm test

desktop-package: build web-build desktop-build ## Package desktop executable and installer
	cd desktop && npm run package

desktop-package-win: build-win web-build desktop-build ## Cross-package Windows desktop zip and unpacked app from Linux
	cd desktop && npm run package:win

e2e: ## Run the Playwright E2E suite against the real stack
	cd web && npm run test:e2e

verify: vet test web-test web-typecheck web-build desktop-test ## Run everything: vet, Go tests, frontend tests, typecheck, frontend build, desktop tests

release: ## Build Linux/Windows packages into release/ (see ./release.sh help)
	./release.sh

clean: ## Remove build artifacts
	rm -rf bin
