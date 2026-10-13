FROM golang:1.26 AS builder

WORKDIR /app

COPY . .

ARG GIT_TAG=dev

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X github.com/shanehull/housecycle/internal/buildinfo.GitTag=${GIT_TAG}" \
    -o /app/housecycle ./cmd/housecycle

FROM scratch

COPY --from=builder /app/housecycle /usr/local/bin/housecycle
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ENV HOUSECYCLE_ADDR=:8080
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/housecycle"]
