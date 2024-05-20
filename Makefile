DOCKER_IMG ?= github_actions_exporter

.PHONY: lint
lint:
	@echo ">> running golangci-lint"
	golangci-lint run ./...

.PHONY: test
test:
	@echo ">> running all tests"
	GO111MODULE=on go test -race  ./...

.PHONY: build
build:
	CGO_ENABLED=0 go build -o github-actions-exporter .