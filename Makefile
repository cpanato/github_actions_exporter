DOCKER_IMG ?= form3tech/github_actions_exporter

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

.PHONY: publish
publish: build
	@echo "==> Building docker image..."
	docker build . -t $(DOCKER_IMG):$(DOCKER_TAG)
	@echo "==> Logging in to the docker registry..."
	echo "$(DOCKER_PASSWORD)" | docker login -u "$(DOCKER_USERNAME)" --password-stdin
	@echo "==> Pushing built image..."
	docker push $(DOCKER_IMG):$(DOCKER_TAG)
	docker tag $(DOCKER_IMG):$(DOCKER_TAG) $(DOCKER_IMG):latest
	docker push $(DOCKER_IMG):latest
