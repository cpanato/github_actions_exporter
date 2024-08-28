all:: lint test build

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
	GOOS=linux GOARCH=amd64 CG_ENABLED=1 go build -o github-actions-exporter .
