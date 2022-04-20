all:: lint test

.PHONY: lint
lint:
	@echo ">> running golangci-lint"
	golangci-lint run ./...

.PHONY: test
test:
	@echo ">> running all tests"                                                                                                                                                                                                  │
	GO111MODULE=on go test -race  ./...
