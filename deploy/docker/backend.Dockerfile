ARG GO_VERSION=1.23
ARG GO_IMAGE=golang:${GO_VERSION}-alpine
ARG RUNTIME_IMAGE=alpine:3.20
ARG APK_REPOSITORY=https://mirrors.aliyun.com/alpine

FROM ${GO_IMAGE} AS build

ARG APP_VERSION=local
ARG APK_REPOSITORY
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG ALL_PROXY
ARG NO_PROXY

ENV GOPROXY=${GOPROXY} \
    GOSUMDB=${GOSUMDB} \
    HTTP_PROXY=${HTTP_PROXY} \
    HTTPS_PROXY=${HTTPS_PROXY} \
    ALL_PROXY=${ALL_PROXY} \
    NO_PROXY=${NO_PROXY}

WORKDIR /src/backend

RUN sed -i "s|https://dl-cdn.alpinelinux.org/alpine|${APK_REPOSITORY}|g" /etc/apk/repositories \
    && apk add --no-cache ca-certificates git tzdata

COPY backend/go.mod backend/go.sum ./

RUN go mod download

COPY backend/ ./

RUN go mod tidy && go mod download

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${APP_VERSION}" \
    -o /out/api ./cmd/api \
    && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${APP_VERSION}" \
    -o /out/worker ./cmd/worker \
    && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${APP_VERSION}" \
    -o /out/migrate ./cmd/migrate \
    && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${APP_VERSION}" \
    -o /out/bootstrap-admin ./cmd/bootstrap-admin

FROM ${RUNTIME_IMAGE} AS runtime

ARG APP_VERSION=local
ARG APK_REPOSITORY
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG ALL_PROXY
ARG NO_PROXY

LABEL org.opencontainers.image.title="ov-dash-backend" \
      org.opencontainers.image.version="${APP_VERSION}" \
      org.opencontainers.image.description="ov-dash Go API and worker runtime"

ENV APP_ENV=production \
    TZ=Asia/Shanghai \
    HTTP_PROXY=${HTTP_PROXY} \
    HTTPS_PROXY=${HTTPS_PROXY} \
    ALL_PROXY=${ALL_PROXY} \
    NO_PROXY=${NO_PROXY}

RUN sed -i "s|https://dl-cdn.alpinelinux.org/alpine|${APK_REPOSITORY}|g" /etc/apk/repositories \
    && apk add --no-cache ca-certificates openssh-client tzdata wget \
    && addgroup -S app \
    && adduser -S -D -H -h /app -s /sbin/nologin -G app app \
    && mkdir -p /app \
    && chown -R app:app /app

WORKDIR /app

COPY --from=build --chown=app:app /out/api /app/api
COPY --from=build --chown=app:app /out/worker /app/worker
COPY --from=build --chown=app:app /out/migrate /app/migrate
COPY --from=build --chown=app:app /out/bootstrap-admin /app/bootstrap-admin
COPY --chown=app:app backend/migrations /migrations

USER app

EXPOSE 8080
