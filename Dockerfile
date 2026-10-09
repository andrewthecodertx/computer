# Pinned patch level; keep in lockstep with the tests service in docker-compose.yml.
FROM golang:1.23.12-alpine3.22 AS build
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/computer .

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /out/computer /computer
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/computer"]
