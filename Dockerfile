FROM golang:1.26.0-bookworm AS builder

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/token-refresher ./cmd/token-refresher

FROM node:22-bookworm-slim

ARG CODEX_VERSION=0.118.0

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && npm install -g @openai/codex@${CODEX_VERSION} \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/token-refresher /usr/local/bin/token-refresher

USER node

ENTRYPOINT ["/usr/local/bin/token-refresher"]
