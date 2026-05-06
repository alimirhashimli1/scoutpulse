# Stage 1: Build
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy the entire workspace context (needed for libs/auth)
COPY . .

# Build the identity-svc
# We need to build from the root context or adjust paths because of the local replace in go.mod
RUN cd apps/identity-svc && go mod download && go build -o /identity-svc main.go

# Stage 2: Final
FROM alpine:latest

WORKDIR /

COPY --from=builder /identity-svc /identity-svc

EXPOSE 8080

CMD ["/identity-svc"]
