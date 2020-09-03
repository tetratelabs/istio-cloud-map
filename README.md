# Istio Cloud Map Operator

This repo contains an operator for syncing Cloud Map data into Istio by pushing ServiceEntry CRDs to the Kube API server.

## Deploying to your Kubernetes cluster

1. Create an [AWS IAM identity](https://docs.aws.amazon.com/IAM/latest/UserGuide/introduction_access-management.html) with read access to AWS Cloud Map for the operator to use.
2. Edit the configuration in `kubernetes/aws-config.yaml`. There are two pieces:
    - A Kubernetes secret with the Access Key ID and Secret Access Key of the identity you just created in the namespace you want to deploy the Istio Cloud Map Operator:
      ```yaml
      apiVersion: v1
      kind: Secret
      metadata:
        name: aws-creds
      type: Opaque
      data:
        access-key-id: <base64-encoded-IAM-access-key-id> # EDIT ME
        secret-access-key: <base64-encoded-IAM-secret-access-key> # EDIT ME
      ```
    - Edit the `aws-config` config map to choose the AWS Cloud Map region to sync with:
      ```yaml
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: aws-config
      data:
        aws-region: us-west-2 # EDIT ME
      ```
4. Deploy the Istio Cloud Map Operator:
    ```bash
    $ kubectl apply -f kubernetes/rbac.yaml -f kubernetes/aws-config.yaml  -f kubernetes/deployment.yaml
    ```
5. Verify that your ServiceEntries have been populated with the information in Cloud Map; there should be one ServiceEntry for every service in Cloud Map:
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

> Note: If you need to be able to resolve your services via DNS (as opposed to making the requests to a random IP and setting the Host header), either enable DNS propagation in your VPC peering configuration or install the [Istio CoreDNS plugin](https://github.com/istio-ecosystem/istio-coredns-plugin).

## Configuring the Operator

`istio-cloud-map serve` flags:
| Flag | Type | Description |
|------|------|-------------|
| `--aws-access-key-id` | string | AWS Access Key ID to use to connect to Cloud Map. Use flags for both this and `--aws-secret-access-key` OR use the environment variables `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`. Flags and env vars cannot be mixed |
| `--aws-region` | string | AWS Region to connect to Cloud Map. Use this OR the environment variable `AWS_REGION` |
| `--aws-secret-access-key` | string |  AWS Secret Access Key to use to connect to Cloud Map. Use flags for both this and `--aws-access-key-id` OR use the environment variables `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`. Flags and env vars cannot be mixed |
| `--debug` | boolean | if true, enables more logging (default true) |
| `-h`, `--help` | none | help for serve |
| `--id` | string | ID of this instance; instances will only ServiceEntries marked with their own ID. (default "istio-cloud-map-operator") |
| `--kube-config` | string | kubeconfig location; if empty the server will assume it's in a cluster; for local testing use ~/.kube/config |
| `--namespace` | string | If provided, the namespace this operator publishes ServiceEntries to. If no value is provided it will be populated from the `PUBLISH_NAMESPACE` environment variable. If all are empty, the operator will publish into the namespace it is deployed in |

## Building

Build with the makefile by:
```bash
make   # or `make build`
```

And produce docker containers via:
```bash
make docker-build
make docker-push
```
You can override the hub and tag using the `REGISTRY` and `TAG` environment variables:

```bash
env REGISTRY=gcr.io/tetratelabs TAG=v0.1 \
    make docker-push
```


Alternatively, just use `go`:
```bash
go build -o istio-cloud-map github.com/tetratelabs/istio-cloud-map/cmd/istio-cloud-map
``` 

## Running Locally

To run locally:
```bash
# Be sure to set the ENV vars:
#   AWS_SECRET_ACCESS_KEY
#   AWS_ACCESS_KEY_ID
#   AWS_REGION

make run
# or
make docker-run
```

or via go:
```bash
go build -o istio-cloud-map github.com/tetratelabs/istio-cloud-map/cmd/istio-cloud-map

./istio-cloud-map serve \
    --kube-config ~/.kube/config \
    --aws-access-key-id "my access key ID" \
    --aws-secret-access-key "my secret access key" \
    --aws-region "us-east-2"
```

In particular the controller needs its `--kube-config` flag set to talk to the remote API server. If no flag is set, the controller assumes it is deployed into a Kubernetes cluster and attempts to contact the API server directly. Similarly, we need AWS credentials; if the flags aren't set we search the `AWS_SECRET_ACCESS_KEY`, `AWS_ACCESS_KEY_ID`, and `AWS_REGION` environment variables.


To run go tests locally:
```bash
docker run -d -p 8500:8500 consul:1.8.3 # setup local consul for testing pkg/consul

go test ./... -v -race
```
