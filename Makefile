# override to push to a different registry or tag the image differently
CONTAINER_REGISTRY ?= gcr.io/tetratelabs
CONTAINER_TAG ?= v0.1

# Make sure we pick up any local overrides.
-include .makerc

build: istio-cloud-map
istio-cloud-map:
	go build -o istio-cloud-map github.com/tetratelabs/istio-cloud-map/cmd/istio-cloud-map
	chmod +x istio-cloud-map

run: istio-cloud-map
	./istio-cloud-map serve --kube-config ~/.kube/config


build-static: docker/istio-cloud-map-static

docker/istio-cloud-map-static:
	GOOS=linux go build \
		-a --ldflags '-extldflags "-static"' -tags netgo -installsuffix netgo \
		-o docker/istio-cloud-map-static github.com/tetratelabs/istio-cloud-map/cmd/istio-cloud-map
	chmod +x docker/istio-cloud-map-static

docker-build: docker/istio-cloud-map-static
	docker build -t $(REGISTRY)/istio-cloud-map:$(TAG) docker/

docker-push: docker-build
	docker push $(REGISTRY)/istio-cloud-map:$(TAG)

docker-run: docker-build
	# local run, mounting kube config into the container and allowing it to use a host network to access the remote cluster
	@docker run \
		-v ~/.kube/config:/etc/istio-cloud-map/kube-config \
		--network host \
		$(REGISTRY)/istio-cloud-map:$(TAG) serve --kube-config /etc/istio-cloud-map/kube-config

clean:
	rm -f istio-cloud-map
