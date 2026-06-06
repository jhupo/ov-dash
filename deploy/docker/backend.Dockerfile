ARG GO_VERSION=1.23
ARG GO_IMAGE=golang:${GO_VERSION}-alpine
ARG RUNTIME_IMAGE=alpine:3.20

FROM ${GO_IMAGE} AS build

ARG APP_VERSION=local
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn

ENV GOPROXY=${GOPROXY} \
    GOSUMDB=${GOSUMDB}

WORKDIR /src/backend

RUN apk add --no-cache ca-certificates git tzdata

COPY backend/go.mod backend/go.sum ./

RUN go mod download

COPY backend/ ./

RUN go mod tidy && go mod download

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${APP_VERSION}" \
    -o /out/api ./cmd/api \
    && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${APP_VERSION}" \
    -o /out/worker ./cmd/worker

FROM ${RUNTIME_IMAGE} AS runtime

ARG APP_VERSION=local

LABEL org.opencontainers.image.title="ov-dash-backend" \
      org.opencontainers.image.version="${APP_VERSION}" \
      org.opencontainers.image.description="ov-dash Go API and worker runtime"

ENV APP_ENV=production \
    TZ=Asia/Shanghai \
    PYTHON_BIN=python3 \
    PYTHON_SCRIPTS_DIR=/app/scripts

RUN apk add --no-cache ca-certificates docker-cli docker-cli-compose git openssh-client python3 py3-pip tzdata wget \
    && addgroup -S app \
    && adduser -S -D -H -h /app -s /sbin/nologin -G app app \
    && mkdir -p /app/scripts \
    && chown -R app:app /app

WORKDIR /app

COPY --from=build --chown=app:app /out/api /app/api
COPY --from=build --chown=app:app /out/worker /app/worker
COPY --chown=app:app backend/scripts /app/scripts

USER app

EXPOSE 8080
