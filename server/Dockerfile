# ORBIT API server. Build from the repository root:
#   docker compose build api
FROM golang:1.26-alpine AS build
WORKDIR /src

COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/orbit-api ./cmd/api

FROM alpine:3.21
RUN apk add --no-cache ca-certificates wget \
    && adduser -D -u 10001 orbit
COPY --from=build /out/orbit-api /usr/local/bin/orbit-api

USER orbit
EXPOSE 8080
ENTRYPOINT ["orbit-api"]