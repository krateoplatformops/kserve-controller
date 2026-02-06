ARCH?=amd64
REPO?=#your repository here 
VERSION?=0.1

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build -o ./bin/kserve-controller main.go

container:
	docker build -t $(REPO)kserve-controller:$(VERSION) .
	docker push $(REPO)kserve-controller:$(VERSION)
	docker build -t $(REPO)kserve-sklearn-krateo-runner:$(VERSION) ./runners/krateo
	docker push $(REPO)kserve-sklearn-krateo-runner:$(VERSION)
	docker build -t $(REPO)kserve-krateo-ttm:$(VERSION) ./models
	docker push $(REPO)kserve-krateo-ttm:$(VERSION)

container-multi:
	docker buildx build --tag $(REPO)kserve-controller:$(VERSION) --push --platform linux/amd64,linux/arm64 .
	docker buildx build --tag $(REPO)kserve-krateo-runner-iris:$(VERSION) --push --platform linux/amd64,linux/arm64 ./runners/krateo-iris
	docker buildx build --tag $(REPO)kserve-krateo-runner-ttm:$(VERSION) --push --platform linux/amd64,linux/arm64 ./runners/krateo-ttm
	docker buildx build --tag $(REPO)kserve-krateo-runner-test:$(VERSION) --push --platform linux/amd64,linux/arm64 ./runners/test
	docker buildx build --tag $(REPO)kserve-krateo-ttm:$(VERSION) --push --platform linux/amd64,linux/arm64 ./models
