FROM node:24.18.0-alpine AS frontend-build

WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.26.5-alpine AS backend-build

WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM alpine:3.22

ARG APP_VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 gamegen \
    && adduser -S -D -H -u 10001 -G gamegen gamegen

WORKDIR /app
COPY --from=backend-build /out/api /out/worker /out/migrate /app/
COPY backend/db/migrations/ /app/migrations/
COPY --from=frontend-build /src/frontend/dist/ /app/public/

ENV WEB_STATIC_DIR=/app/public \
    MIGRATIONS_PATH=/app/migrations

LABEL org.opencontainers.image.title="game-gen" \
      org.opencontainers.image.version="${APP_VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}"

USER 10001:10001
EXPOSE 8080 8081
ENTRYPOINT ["/app/api"]
