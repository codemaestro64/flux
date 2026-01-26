# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy dependency files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the server binary
RUN CGO_ENABLED=0 GOOS=linux go build -o flux-server ./cmd/server/main.go

# Final stage
FROM alpine:latest

WORKDIR /root/

# Install ca-certificates for secure communication if needed
RUN apk --no-cache add ca-certificates

# Copy binary from builder
COPY --from=builder /app/flux-server .

# Expose Raft (7000) and gRPC (50051) ports
EXPOSE 7000 50051

ENTRYPOINT ["./flux-server"]