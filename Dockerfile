# Build stage
FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /llm-proxy .

# Runtime stage
FROM alpine:3.20
COPY --from=builder /llm-proxy /usr/local/bin/llm-proxy
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/llm-proxy"]
