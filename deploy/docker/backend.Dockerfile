ARG GO_VERSION=1.23
ARG GO_IMAGE=golang:${GO_VERSION}-alpine
ARG NODE_VERSION=22
ARG NODE_IMAGE=node:${NODE_VERSION}-alpine
ARG RUNTIME_IMAGE=alpine:3.20
ARG APK_REPOSITORY=https://mirrors.aliyun.com/alpine

FROM ${NODE_IMAGE} AS frontend-deps

ARG NPM_CONFIG_REGISTRY=https://registry.npmmirror.com

ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0 \
    NPM_CONFIG_REGISTRY=${NPM_CONFIG_REGISTRY}

WORKDIR /src/frontend

RUN corepack enable

COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./

RUN --mount=type=cache,id=ov-dash-pnpm-store,target=/root/.local/share/pnpm/store \
    pnpm config set store-dir /root/.local/share/pnpm/store \
    && pnpm config set fetch-retries 5 \
    && pnpm config set fetch-retry-mintimeout 10000 \
    && pnpm config set fetch-retry-maxtimeout 120000 \
    && pnpm config set network-timeout 300000 \
    && pnpm install --frozen-lockfile

FROM frontend-deps AS frontend-build

ARG VITE_API_BASE_URL=/api/v1
ARG VITE_ENABLE_DEVTOOLS=false

ENV VITE_API_BASE_URL=${VITE_API_BASE_URL} \
    VITE_ENABLE_DEVTOOLS=${VITE_ENABLE_DEVTOOLS}

COPY frontend/ ./

RUN pnpm build

FROM ${GO_IMAGE} AS backend-build

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

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${APP_VERSION}" \
    -o /out/launcher ./cmd/launcher \
    && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${APP_VERSION}" \
    -o /out/app ./cmd/app \
    && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${APP_VERSION}" \
    -o /out/migrate ./cmd/migrate \
    && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${APP_VERSION}" \
    -o /out/bootstrap-admin ./cmd/bootstrap-admin

FROM ${RUNTIME_IMAGE} AS runtime

ARG APP_VERSION=local
ARG APK_REPOSITORY
ARG TARGETARCH=amd64
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG ALL_PROXY
ARG NO_PROXY

LABEL org.opencontainers.image.title="ov-dash" \
      org.opencontainers.image.version="${APP_VERSION}" \
      org.opencontainers.image.description="ov-dash launcher and application seed"

ENV APP_ENV=production \
    TZ=Asia/Shanghai \
    UPDATE_RUNTIME_DIR=/opt/ov-dash/runtime \
    FRONTEND_DIR=/opt/ov-dash/runtime/current/web \
    MIGRATIONS_DIR=/opt/ov-dash/runtime/current/migrations \
    HTTP_PROXY=${HTTP_PROXY} \
    HTTPS_PROXY=${HTTPS_PROXY} \
    ALL_PROXY=${ALL_PROXY} \
    NO_PROXY=${NO_PROXY}

RUN sed -i "s|https://dl-cdn.alpinelinux.org/alpine|${APK_REPOSITORY}|g" /etc/apk/repositories \
    && apk add --no-cache ca-certificates openssh-client tzdata wget \
    && addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -h /opt/ov-dash -s /sbin/nologin -G app app \
    && mkdir -p \
        /opt/ov-dash/runtime \
        /opt/ov-dash/seed/${APP_VERSION}/bin \
        /opt/ov-dash/seed/${APP_VERSION}/migrations \
        /opt/ov-dash/seed/${APP_VERSION}/web \
    && printf '%s\n' \
        '{' \
        '  "schema_version": 1,' \
        '  "version": "'"${APP_VERSION}"'",' \
        '  "os": "linux",' \
        '  "arch": "'"${TARGETARCH}"'",' \
        '  "revision": "image",' \
        '  "built_at": "1970-01-01T00:00:00Z",' \
        '  "entrypoints": {' \
        '    "app": "bin/app",' \
        '    "migrate": "bin/migrate",' \
        '    "bootstrap_admin": "bin/bootstrap-admin"' \
        '  },' \
        '  "web_root": "web",' \
        '  "migrations_root": "migrations"' \
        '}' > /opt/ov-dash/seed/${APP_VERSION}/manifest.json \
    && chown -R app:app /opt/ov-dash

COPY --from=backend-build --chown=app:app /out/launcher /opt/ov-dash/launcher
COPY --from=backend-build --chown=app:app /out/app /opt/ov-dash/seed/${APP_VERSION}/bin/app
COPY --from=backend-build --chown=app:app /out/migrate /opt/ov-dash/seed/${APP_VERSION}/bin/migrate
COPY --from=backend-build --chown=app:app /out/bootstrap-admin /opt/ov-dash/seed/${APP_VERSION}/bin/bootstrap-admin
COPY --chown=app:app backend/migrations/ /opt/ov-dash/seed/${APP_VERSION}/migrations/
COPY --from=frontend-build --chown=app:app /src/frontend/dist/ /opt/ov-dash/seed/${APP_VERSION}/web/

WORKDIR /opt/ov-dash

USER app

VOLUME ["/opt/ov-dash/runtime"]

EXPOSE 8080

CMD ["/opt/ov-dash/launcher"]
