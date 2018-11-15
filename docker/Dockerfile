FROM alpine:latest as certs
RUN apk --update add ca-certificates

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ADD istio-route53-static /usr/bin/istio-route53
ENTRYPOINT ["/usr/bin/istio-route53"]
