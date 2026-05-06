# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy the entire workspace to access libs/auth
COPY . .

# Build the football-svc
RUN cd apps/football-svc && go mod download && go build -o /football-svc main.go

# Run stage
FROM alpine:latest

COPY --from=builder /football-svc /football-svc

EXPOSE 8081

CMD ["/football-svc"]
