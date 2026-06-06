ARG NODE_VERSION=22
ARG NODE_IMAGE=node:${NODE_VERSION}-alpine
ARG NGINX_IMAGE=nginx:1.27-alpine
ARG NPM_CONFIG_REGISTRY=https://registry.npmmirror.com

FROM ${NODE_IMAGE} AS deps

ARG NPM_CONFIG_REGISTRY=https://registry.npmmirror.com
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0 \
    NPM_CONFIG_REGISTRY=${NPM_CONFIG_REGISTRY}

WORKDIR /app

RUN corepack enable

COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./

RUN --mount=type=cache,id=ov-dash-pnpm-store,target=/root/.local/share/pnpm/store \
    pnpm config set store-dir /root/.local/share/pnpm/store \
    && pnpm config set fetch-retries 5 \
    && pnpm config set fetch-retry-mintimeout 10000 \
    && pnpm config set fetch-retry-maxtimeout 120000 \
    && pnpm config set network-timeout 300000 \
    && pnpm install --frozen-lockfile

FROM deps AS build

ARG VITE_API_BASE_URL=/api/v1
ARG VITE_ENABLE_DEVTOOLS=false
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL} \
    VITE_ENABLE_DEVTOOLS=${VITE_ENABLE_DEVTOOLS}

COPY frontend/ ./

RUN pnpm build

FROM ${NGINX_IMAGE} AS runtime

COPY deploy/nginx/nginx.conf /etc/nginx/nginx.conf
COPY deploy/nginx/default.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 80
