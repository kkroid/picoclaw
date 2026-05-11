FROM golang:1.25-alpine AS builder

ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}

RUN apk add --no-cache git make nodejs npm
RUN npm install -g pnpm@10.32.1

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY web/frontend/package.json web/frontend/pnpm-lock.yaml ./web/frontend/
RUN cd web/frontend && pnpm install --frozen-lockfile

COPY cmd ./cmd
COPY pkg ./pkg
COPY web/backend ./web/backend
COPY web/frontend/.tanstack ./web/frontend/.tanstack
COPY web/frontend/components.json web/frontend/index.html web/frontend/tsconfig.app.json web/frontend/tsconfig.json web/frontend/tsconfig.node.json web/frontend/vite.config.ts ./web/frontend/
COPY web/frontend/public ./web/frontend/public
COPY web/frontend/scripts ./web/frontend/scripts
COPY web/frontend/src ./web/frontend/src
COPY examples/appfactory ./examples/appfactory
COPY LICENSE ./LICENSE
RUN cd web/frontend && pnpm build:backend
RUN go build -tags goolm,stdjson -ldflags "-s -w" -o /out/oneappfactory ./cmd/oneappfactory
RUN go build -tags goolm,stdjson -ldflags "-s -w" -o /out/oneappfactory-launcher ./cmd/oneappfactory-launcher

FROM alpine:3.21

RUN apk add --no-cache ca-certificates curl docker-cli tzdata

ENV HOME=/root
ENV ONEAPPFACTORY_HOME=/root/.appfactory
ENV ONEAPPFACTORY_CONFIG=/root/.appfactory/config.json

COPY --from=builder /out/oneappfactory /usr/local/bin/oneappfactory
COPY --from=builder /out/oneappfactory-launcher /usr/local/bin/oneappfactory-launcher
COPY docker/entrypoint.sh /entrypoint.sh

RUN chmod +x /entrypoint.sh

EXPOSE 18800

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O /dev/null http://localhost:18800/ || exit 1

ENTRYPOINT ["/entrypoint.sh"]
