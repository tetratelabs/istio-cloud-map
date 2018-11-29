FROM alpine:latest as certs
RUN apk --update add ca-certificates

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ADD istio-cloud-map-static /usr/bin/istio-cloud-map
ENTRYPOINT ["/usr/bin/istio-cloud-map"]
