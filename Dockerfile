FROM golang:1.24 AS builder

WORKDIR /src

COPY go.mod ./
COPY application ./application
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/workorchd ./application/workorchd

FROM alpine:3.22

RUN addgroup -S workorch && adduser -S -G workorch -h /srv/workorch workorch

WORKDIR /srv/workorch

COPY --from=builder /out/workorchd /usr/local/bin/workorchd

RUN mkdir -p /var/lib/workorch && chown -R workorch:workorch /srv/workorch /var/lib/workorch

USER workorch

ENV WORKORCH_SERVER_ADDR=:8080
ENV WORKORCH_REPOSITORY_BACKEND=memory
ENV WORKORCH_DATA_DIR=/var/lib/workorch
ENV WORKORCH_SERVER_READ_HEADER_TIMEOUT=5s
ENV WORKORCH_SERVER_READ_TIMEOUT=15s
ENV WORKORCH_SERVER_WRITE_TIMEOUT=30s
ENV WORKORCH_SERVER_IDLE_TIMEOUT=60s
ENV WORKORCH_SERVER_SHUTDOWN_TIMEOUT=5s
ENV WORKORCH_SERVER_MAX_CONCURRENT_RUNS=1

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/workorchd"]
