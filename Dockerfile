# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /gpt-proxy .

# Runtime stage
FROM alpine:3.20
COPY --from=builder /gpt-proxy /usr/local/bin/gpt-proxy
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/gpt-proxy"]
