# Build stage (Debian-based Go image)
FROM golang:1.24-bullseye AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o llm-proxy ./

# Runtime stage
FROM debian:bullseye-slim
WORKDIR /app
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/llm-proxy /usr/local/bin/llm-proxy

EXPOSE 8080
CMD ["/usr/local/bin/llm-proxy"]
