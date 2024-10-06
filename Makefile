export GOLANGCI_LINT_VERSION := v1.48.0


# Run all the linters
lint: bin/golangci-lint
	# TODO: fix disabled linter issues
	./bin/golangci-lint run ./...
.PHONY: lint

bin/golangci-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s $(GOLANGCI_LINT_VERSION)

# Clean go.mod
go-mod-tidy:
	@go mod tidy -v
	@git diff HEAD
	@git diff-index --quiet HEAD
.PHONY: go-mod-tidy

# Install all the build and lint dependencies
setup:
	go mod download
.PHONY: setup

# Initialize the pre-commit git hook
githooks-init:
	cp .pre-commit .git/hooks/pre-commit
.PHONY: githooks-init

# docker related stuff
docker-compose-up:
	./distributed-test-environment/docker-compose up --build
.PHONY: docker-compose-up

docker-compose-down:
	./distributed-test-environment/docker-compose down
.PHONY: docker-compose-down

