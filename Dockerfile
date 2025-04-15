FROM golang:1.20-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum files to download dependencies
COPY go.mod ./
# Assuming you have a go.sum file
# COPY go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o registry ./cmd/registry

# Start a new stage from scratch
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the pre-built binary from the previous stage
COPY --from=builder /app/harness-cli .

# Copy the config file template
COPY --from=builder /app/config.yaml.example ./config.yaml.example

# Create a volume for configuration and logs
VOLUME ["/root/config", "/root/logs"]

# Command to run
ENTRYPOINT ["./harness-cli"]
CMD ["--help"]
