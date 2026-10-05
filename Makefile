.PHONY: help install backend frontend test coverage fuzz build docker

help: ## Show this help
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

install: ## Install frontend dependencies
	cd frontend && npm ci

backend: ## Run the API on :8080
	cd backend && go run ./cmd/server

frontend: ## Run the UI on :5173 (proxies /api to :8080)
	cd frontend && npm run dev

test: ## Run all unit tests
	cd backend && go vet ./... && go test ./...
	cd frontend && npm run typecheck && npm test

coverage: ## Run tests and write coverage reports
	cd backend && go test ./internal/... -coverprofile=coverage.out \
		&& go tool cover -func=coverage.out \
		&& go tool cover -html=coverage.out -o coverage.html
	cd frontend && npm run coverage

fuzz: ## Fuzz the decimal parser, the calculator and payment plans for 30s each
	cd backend && go test ./internal/calculator -run '^$$' -fuzz=FuzzParse -fuzztime=30s
	cd backend && go test ./internal/calculator -run '^$$' -fuzz=FuzzCalculate -fuzztime=30s
	cd backend && go test ./internal/paymentplan -run '^$$' -fuzz=FuzzBuild -fuzztime=30s

build: ## Build the frontend, then the server binary
	cd frontend && npm run build
	cd backend && go build -o bin/server ./cmd/server

docker: ## Build and run the full stack on http://localhost:8080
	docker build -t calculator .
	docker run --rm -p 8080:8080 calculator
