all:: lint test build

DOCS_IMAGE_VERSION="v1.14.2"

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

.PHONY: docs
docs:
	@docker run \
	--rm \
	--workdir=/helm-docs \
	--volume "$$(pwd):/helm-docs" \
	-u $$(id -u) \
	jnorwood/helm-docs:$(DOCS_IMAGE_VERSION) \
	helm-docs -c ./charts/$* -t ./README.md.gotmpl -o ./README.md
