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
COPY config/application.toml /etc/workorch/application.toml

RUN mkdir -p /var/lib/workorch && chown -R workorch:workorch /srv/workorch /var/lib/workorch /etc/workorch

USER workorch

ENV CONFIG_PATH=/etc/workorch

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/workorchd", "-config-dir", "/etc/workorch"]
