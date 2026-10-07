# computer — Next.js front-end.
# Multi-stage build: install + build in a Node image, run the standalone
# output in a minimal Node image. No shell, no package manager at runtime.
#
# Build:  docker build -t computer-web .
# Run:    docker compose up   (db + app + web)
#
# At runtime the Next server proxies /api/* (except /api/auth/*) to the Go
# backend via API_BASE_URL. /api/auth/* stays local — NextAuth's session, csrf
# and callback handlers live here, not in Go.
#
#   API_BASE_URL=http://go-computer:8080     # Docker DNS
#   DATABASE_URL=postgres://computer:computer@db:5432/computer?sslmode=disable

# ---------- deps ----------
FROM node:22-alpine AS deps
WORKDIR /app
COPY package.json yarn.lock .yarnrc.yml ./
RUN corepack enable && yarn install --frozen-lockfile

# ---------- build ----------
FROM node:22-alpine AS build
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN corepack enable && yarn prisma generate && yarn build

# ---------- runtime ----------
FROM node:22-alpine AS runtime
WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1

# Next standalone output layout:
#   ./next.config.js  ./public  ./package.json
#   ./server.js  ./.next/standalone/...
COPY --from=build /app/next.config.js ./
COPY --from=build /app/public ./public
COPY --from=build /app/package.json ./
COPY --from=build /app/.next/standalone ./

EXPOSE 3000

ENV PORT=3000
ENV HOSTNAME=0.0.0.0
ENV API_BASE_URL="http://go-computer:8080"
ENV DATABASE_URL="postgres://computer:computer@db:5432/computer?sslmode=disable"

CMD ["node", "server.js"]