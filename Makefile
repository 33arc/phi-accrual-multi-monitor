GOLANGCI_LINT_VERSION := v1.48.0
PORTS := 9000 9001 9002
ENDPOINT := /metrics
SERVER := server1

.PHONY: all setup lint go-mod-tidy githooks-init run docker-compose-down get_metrics help

all: get_metrics

# Development setup
setup:
	go mod download

githooks-init:
	cp .pre-commit .git/hooks/pre-commit

# Linting and code quality
lint: bin/golangci-lint
	./bin/golangci-lint run ./...

bin/golangci-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s $(GOLANGCI_LINT_VERSION)

go-mod-tidy:
	@go mod tidy -v
	@git diff HEAD
	@git diff-index --quiet HEAD

# Docker operations
run:
	docker-compose -f ./distributed-test-environment/docker-compose.yml up --build

docker-compose-down:
	docker-compose -f ./distributed-test-environment/docker-compose.yml down

# Metrics collection
get_metrics:
	@echo "Fetching metrics for $(SERVER) from all monitors..."
	@for port in $(PORTS); do \
		echo "Monitor on port $$port:"; \
		curl -s "http://localhost:$$port$(ENDPOINT)" | grep -i "$(SERVER)"; \
		echo; \
	done

# Help
help:
	@echo "Available targets:"
	@echo "  all              : Fetch metrics from all monitors and display $(SERVER) info (default)"
	@echo "  setup            : Install all the build and lint dependencies"
	@echo "  lint             : Run all the linters"
	@echo "  go-mod-tidy      : Clean go.mod"
	@echo "  githooks-init    : Initialize the pre-commit git hook"
	@echo "  run              : Start the Docker environment"
	@echo "  docker-compose-down : Stop the Docker environment"
	@echo "  get_metrics      : Fetch metrics from all monitors"
	@echo "  help             : Show this help message"
