# override to push to a different registry or tag the image differently
CONTAINER_REGISTRY ?= gcr.io/tetratelabs
CONTAINER_TAG ?= v0.1

deps: $(DEP)
	@echo "Fetching dependencies..."
	dep ensure -v

$(DEP):
	@echo "dep not found, installing..."
	@go get github.com/golang/dep/cmd/dep


build: istio-route53
istio-route53:
	go build -o istio-route53 github.com/tetratelabs/istio-route53/cmd/istio-route53
	chmod +x istio-route53

run: istio-route53
	./istio-route53 serve --kube-config ~/.kube/config


build-static: docker/istio-route53-static
docker/istio-route53-static:
	GOOS=linux go build \
		-a --ldflags '-extldflags "-static"' -tags netgo -installsuffix netgo \
		-o docker/istio-route53-static github.com/tetratelabs/istio-route53/cmd/istio-route53
	chmod +x docker/istio-route53-static


docker-build: docker/istio-route53-static
	docker build -t $(CONTAINER_REGISTRY)/istio-route53:$(CONTAINER_TAG) -f docker/Dockerfile .

docker-push: docker-build
	docker push $(CONTAINER_REGISTRY)/istio-route53:$(CONTAINER_TAG)

docker-run: docker-build
	# local run, mounting kube config into the container and allowing it to use a host network to access the remote cluster
	@docker run \
		-v ~/.kube/config:/etc/istio-route53/kube-config \
		--network host \
		gcr.io/tetratelabs/istio-route53:v0.1 serve --kube-config /etc/istio-route53/kube-config