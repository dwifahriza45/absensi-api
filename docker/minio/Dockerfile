FROM golang:1.27.3-alpine AS server-build
RUN apk add --no-cache ca-certificates git
# Official security release; build from source because upstream no longer publishes it as an image.
RUN go install github.com/minio/minio@RELEASE.2025-10-15T17-29-55Z

FROM golang:1.27.3-alpine AS client-build
RUN apk add --no-cache ca-certificates git
RUN go install github.com/minio/mc@RELEASE.2025-08-13T08-35-41Z

FROM alpine:3.22 AS server
RUN apk add --no-cache ca-certificates curl tzdata && \
    addgroup -S -g 10001 minio && adduser -S -D -H -u 10001 -G minio minio && \
    mkdir -p /data && chown minio:minio /data
COPY --from=server-build /go/bin/minio /usr/local/bin/minio
ENV HOME=/tmp
USER 10001:10001
EXPOSE 9000 9001
ENTRYPOINT ["minio"]
CMD ["server", "/data", "--address", ":9000", "--console-address", ":9001", "--certs-dir", "/tmp/certs"]

FROM alpine:3.22 AS client
RUN apk add --no-cache ca-certificates && \
    addgroup -S -g 10001 minio && adduser -S -D -H -u 10001 -G minio minio
COPY --from=client-build /go/bin/mc /usr/local/bin/mc
COPY docker/minio/init.sh /usr/local/bin/init-minio
RUN chmod 755 /usr/local/bin/init-minio
ENV HOME=/tmp
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/init-minio"]
