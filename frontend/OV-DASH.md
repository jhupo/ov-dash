# OV Dash frontend

This directory is based on `satnaing/shadcn-admin` and is kept as the isolated
frontend workspace for OV Dash.

## Local development

```bash
cd frontend
pnpm install
pnpm dev
```

The application is configured as a Vite React app with TanStack Router,
TanStack Query, Tailwind CSS, shadcn/ui, Radix UI, and Axios.

## Runtime configuration

Copy `.env.example` to `.env.local` and set values for the target environment.

| Variable | Default | Purpose |
| --- | --- | --- |
| `VITE_API_BASE_URL` | `/api/v1` | Go backend API base URL. |
| `VITE_API_WITH_CREDENTIALS` | `false` | Send cookies/credentials with API requests. |
| `VITE_CLERK_PUBLISHABLE_KEY` | empty | Optional Clerk integration from upstream template. |

Use `src/lib/http-client.ts` for shared API calls and keep endpoint-specific
code under `src/services` or feature-local service modules.

## Container build

```bash
cd frontend
docker build -t ov-dash-frontend .
docker run --rm -p 8080:80 ov-dash-frontend
```

Build-time API overrides can be passed as Docker build args:

```bash
docker build \
  --build-arg VITE_API_BASE_URL=/api/v1 \
  --build-arg VITE_API_WITH_CREDENTIALS=false \
  -t ov-dash-frontend .
```

The container serves the built SPA with Nginx and exposes `/healthz` for
frontend container health checks.
