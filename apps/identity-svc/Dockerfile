# Stage 1: Builder
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /app

# Copy the entire repository to resolve local library dependencies
# (libs/auth, libs/db, libs/platform) via this module's replace directives.
COPY . .

# Build from the module alone rather than the go.work workspace, so the image
# depends only on this service's go.mod/go.sum and stays reproducible.
ENV GOWORK=off

# Download dependencies for identity-svc
WORKDIR /app/apps/identity-svc
RUN go mod download

# Build the binary
# -ldflags="-s -w" reduces binary size by removing debug symbols
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/identity-svc ./cmd/server/main.go

# Stage 2: Runtime
# Using scratch or alpine for a minimal production image
FROM alpine:3.19

# Add a non-privileged user for security
RUN adduser -D scoutuser
USER scoutuser

WORKDIR /home/scoutuser/

# Copy the binary from the builder stage
COPY --from=builder /app/identity-svc .

# Expose the port identity-svc listens on
EXPOSE 8080

# Run the application
ENTRYPOINT ["./identity-svc"]
