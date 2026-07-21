# Application Onboarding Progress

This file tracks the parts of the football database application explained during the junior-developer onboarding.

## Explained

- Repository structure and service boundaries
  - This is a Go monorepo containing independently deployable services under `apps/`.
  - `identity-svc` owns users and authentication; `football-svc` owns football-domain data.
  - `libs/auth` and `libs/db` are shared Go modules; `deploy/` holds infrastructure configuration.
  - The Angular frontend is planned but not yet implemented.
- Local development environment and Docker services
  - The root `docker-compose.yml` runs both Go services, a separate PostgreSQL database for each service, and a frontend container on one internal Docker network.
  - Service addresses from the host are identity `localhost:8080`, football `localhost:8081`, frontend `localhost:4200`, and football PostgreSQL `localhost:5434`.
  - Migrations run only when a PostgreSQL data volume is first initialized; named volumes retain data across normal restarts.
  - `make up`, `make logs`, `make down`, `make build`, and `make test-all` are the primary developer commands. `make clean` also removes database data.
- Backend application architecture and API flow
  - Backend requests follow route → handler → service → repository → PostgreSQL.
  - Each service starts in `main.go`, where database connections and layer dependencies are assembled explicitly.
  - Identity login validates a bcrypt password hash and returns a JWT; protected football write routes require that JWT in the `Authorization: Bearer <token>` header.
  - Middleware validates the token, while service-level rules enforce roles and editor team permissions.

## Remaining

- Frontend application architecture and UI flow
- Database schema, migrations, and data access
- Testing, smoke tests, and quality checks
- Deployment and day-to-day development workflow
