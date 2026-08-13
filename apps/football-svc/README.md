# Football Service

The **Football Service** is the core domain service of the ScoutPulse Micro ecosystem. It handles all football-related data, including leagues, teams, players, and transfers.

## Tech Stack
- **Language:** Go (Golang)
- **Database:** PostgreSQL
- **Architecture:** REST API (External), gRPC (Internal - Planned)
- **Security:** Integrated with `libs/auth` for JWT validation.

## Features
- **Public Data:** Unauthenticated access to player lists and general football information.
- **Admin Access:** Secure endpoints for data modification, restricted to users with the `admin` role.
- **Health Monitoring:** `/health` endpoint for container orchestration readiness checks.

## API Endpoints

| Endpoint | Method | Access | Description |
|----------|--------|--------|-------------|
| `/health` | `GET` | Public | Service status check |
| `/players` | `GET` | Public | List of players (Sample data) |
| `/admin/players` | `GET/POST` | Admin Only | Administrative player management |

## Environment Variables
The service uses the following environment variables (defined via `libs/db`):

- `DB_HOST`: Database server host (default: `localhost`)
- `DB_PORT`: Database server port (default: `5432`)
- `DB_USER`: Database username
- `DB_PASSWORD`: Database password
- `DB_NAME`: Database name

## Getting Started
To run the service locally:
1. Ensure a PostgreSQL instance is running.
2. Set the required environment variables.
3. Run `go run main.go`.
