# Istio Cloud Map Operator

This repo contains an operator for syncing Route53 data into Istio by pushing ServiceEntry CRDs to the Kube API server.

## Deploying to your Kubernetes cluster

1. Create an [AWS IAM identity](https://docs.aws.amazon.com/IAM/latest/UserGuide/introduction_access-management.html) with read access to AWS Cloud Map.
2. Create a Kubernetes secret with the Access Key ID and Secret Access Key of the identity you just created in the namespace you want to deploy the Istio Cloud Map Operator:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: aws-credz
  namespace: istio-system
type: Opaque
data:
  access-key-id: <base64-encoded-IAM-access-key-id>
  secret-access-key: <base64-encoded-IAM-secret-access-key>
```
3. Deploy the Istio Cloud Map Operator:
```bash
$ kubectl apply -f kubernetes/deployment.yaml -f kubernetes/rbac.yaml
```
4. Verify that your Service Entries have been populated with the information in Cloud Map
```bash
$ kubectl get serviceentries
NAME                                       CREATED AT
cloudmap-dev.null.demo.tetrate.io          17h
cloudmap-test-server.cloudmap.tetrate.io   17h
```
```yaml
$ kubectl get serviceentries cloudmap-test-server.cloudmap.tetrate.io -o yaml
apiVersion: networking.istio.io/v1alpha3
kind: ServiceEntry
metadata:
  name: cloudmap-test-server.cloudmap.tetrate.io
  namespace: default
spec:
  addresses:
  - 172.31.37.168
  endpoints:
  - address: 172.31.37.168
    ports:
      http: 80
      https: 443
  hosts:
  - test-server.cloudmap.tetrate.io
  ports:
  - name: http
    number: 80
    protocol: HTTP
  - name: https
    number: 443
    protocol: HTTPS
  resolution: STATIC
```

>Note: If you need to be able to resolve your services via DNS (as opposed to making the requests to a random IP and setting the Host header), install the [Istio CoreDNS plugin](https://github.com/istio-ecosystem/istio-coredns-plugin).

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
go build -o istio-route53 github.com/tetratelabs/istio-route53/cmd/istio-route53
``` 

## Running Locally

To run locally:
```bash
make run
# or
make docker-run
```

or via go:
```bash
go build -o istio-route53 github.com/tetratelabs/istio-route53/cmd/istio-route53
./istio-route53 serve --kube-config ~/.kube/config
```

In particular the controller needs its `--kube-config` flag set to talk to the remote API server. If no flag is set, the controller assumes it is deployed into a Kubernetes cluster and attempts to contact the API server directly.
