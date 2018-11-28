# Istio Cloud Map Operator

This repo contains an operator for syncing Cloud Map data into Istio by pushing ServiceEntry CRDs to the Kube API server.

## Building

Build with the makefile by:
```bash
make deps # only needs to be done once
make      # or `make build`
```

Run with
```bash
make run
```

And produce docker containers via:
```bash
make docker-build
make docker-push
```
You can override the hub and tag using the `CONTAINER_REGISTRY` and `CONTAINER_TAG` environment variables:


```bash
env CONTAINER_REGISTRY=gcr.io/tetratelabs CONTAINER_TAG=v0.1 \
    make docker-push
```


Alternatively, just use `go`:
```bash
dep ensure
go build -o istio-cloud-map github.com/tetratelabs/istio-cloud-map/cmd/istio-cloud-map
``` 

## Running

To run locally:
```bash
make run
# or
make docker-run
```

or via go:
```bash
go build -o istio-cloud-map github.com/tetratelabs/istio-cloud-map/cmd/istio-cloud-map
./istio-cloud-map serve --kube-config ~/.kube/config
```

In particular the controller needs its `--kube-config` flag set to talk to the remote API server. If no flag is set, the controller assumes it is deployed into a Kubernetes cluster and attempts to contact the API server directly.
