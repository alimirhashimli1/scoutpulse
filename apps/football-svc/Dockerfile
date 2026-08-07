# Stage 1: Builder
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

# Copy the entire repository to resolve local library dependencies
# (libs/auth, libs/db, libs/platform) via this module's replace directives.
COPY . .

# Build from the module alone rather than the go.work workspace, so the image
# depends only on this service's go.mod/go.sum and stays reproducible.
ENV GOWORK=off

WORKDIR /app/apps/football-svc
RUN go mod download

# -ldflags="-s -w" strips debug symbols to reduce binary size.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/football-svc .

# Stage 2: Runtime
FROM alpine:3.19

# Run as a non-privileged user.
RUN adduser -D scoutuser
USER scoutuser

WORKDIR /home/scoutuser/

COPY --from=builder /app/football-svc .

EXPOSE 8081

ENTRYPOINT ["./football-svc"]
