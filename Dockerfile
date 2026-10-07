# computer — Go backend.
# Multi-stage build: compile in a Go image, run the binary in a minimal
# distroless-like image. No shell, no package manager — sandboxed runtime.
#
# Build:  docker build -t go-computer .
# Run:    docker compose up

# ---------- build stage ----------
FROM golang:1.23-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/go-computer .

# ---------- runtime stage ----------
FROM scratch

COPY --from=build /out/go-computer /go-computer

# Ports the app listens on.
EXPOSE 8080

# Env vars expected at runtime (see computer_porting_guide.md section 7):
#   DATABASE_URL   postgres://computer:computer@db:5432/computer?sslmode=disable
#   NEXTAUTH_SECRET / AUTH_SECRET, NEXTAUTH_URL, optional OIDC_*, ADMIN_EMAILS
ENV DATABASE_URL="postgres://computer:computer@db:5432/computer?sslmode=disable"

ENTRYPOINT ["/go-computer"]