# Stage 1: Build binary
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copy dependency definition
COPY go.mod ./

# Copy source code
COPY . .

# Compile a highly optimized statically linked binary without debug symbols
ARG VERSION=v0.1.0
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /app/api \
    cmd/api/main.go

# Stage 2: Final minimal security hardened image
FROM gcr.io/distroless/static-debian12

WORKDIR /

# Copy compiler artifact
COPY --from=builder /app/api /api

# Expose target service port
EXPOSE 8080

# Execute as standard nonroot UID to minimize host vulnerability surface
USER nonroot:nonroot

ENTRYPOINT ["/api"]
