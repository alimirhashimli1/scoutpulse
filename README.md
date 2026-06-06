# ScoutPulse Micro ⚽

ScoutPulse Micro is a high-performance football database platform built using a modern microservices architecture. It aims to provide comprehensive data on leagues, teams, players, and transfers with a focus on scalability and security.

## Project Structure

```text
/apps
  ├── football-svc    # Core domain logic (Go)
  ├── identity-svc    # Auth & User management (Go)
  └── frontend        # UI Dashboard (Angular - Planned)
/libs
  ├── auth            # Shared JWT & RBAC library
  └── db              # Shared Database connection library
/deploy               # Docker Compose & Infrastructure
```

## Work Completed So Far 🚀

### 🏗️ Foundation & Infrastructure
- **Workspace Setup:** Initialized a Go-based monorepo with proper modularity.
- **Shared Libraries:**
    - Created `libs/db` for standardized PostgreSQL connections.
    - Created `libs/auth` for centralized JWT generation, validation, and RBAC middleware.
- **DevOps:** Established Docker containerization and a GitHub Actions CI/CD pipeline.

### 🔐 Identity Service (Phase 2 - COMPLETED)
- **User Management:** Implemented registration and login with support for both Email and Username.
- **Security:** Built a robust JWT-based authentication system.
- **Persistence:** Set up PostgreSQL with schema migrations for user data.
- **Testing:** Achieved high reliability with unit tests and integration tests using Testcontainers.
- **Health Check:** Added monitoring endpoints for service health.

### ⚽ Football Service (Phase 3 - IN PROGRESS)
- **Service Core:** Initialized the service and integrated shared libraries.
- **Auth Integration:** Successfully implemented the shared Auth Middleware to protect sensitive routes.
- **Routing:** Created public routes for general data and protected routes for administrators.
- **Health Check:** Implemented `/health` endpoint.

## Architecture Highlights
- **Microservices:** Decoupled services communicating via REST and gRPC.
- **RBAC:** Fine-grained Role-Based Access Control (ADMIN, EDITOR, USER).
- **Database per Service:** Isolated PostgreSQL schemas for each service.
- **Containerization:** Ready for Docker-based deployment.

## Getting Started
Please refer to the `README.md` files within each service directory in `/apps` for specific setup instructions.
