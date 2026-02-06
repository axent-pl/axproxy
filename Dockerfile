FROM golang:1.25.7-bookworm AS builder


ENV CGO_ENABLED=1 \
    GO111MODULE=on

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential pkg-config libkrb5-dev ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN mkdir -p /out && go build -trimpath -ldflags="-s -w" -o /out/app .

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libkrb5-3 libgssapi-krb5-2 ca-certificates && \
    rm -rf /var/lib/apt/lists/*

ENV KRB5CCNAME=MEMORY:
ENV KRB5_CONFIG=/etc/krb5.conf
ENV KRB5_CLIENT_KTNAME=/run/secrets/proxy.keytab

RUN useradd -m -u 10001 appuser
WORKDIR /app

COPY --from=builder /out/app /app/app
COPY assets/ /app/assets/
RUN chown -R appuser:appuser /app
USER appuser

CMD ["/app/app"]