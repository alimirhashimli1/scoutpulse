# ScoutPulse Micro - Project Specification & Roadmap

## 1. Overview
ScoutPulse Micro is a high-performance football database platform (similar to Transfermarkt) built using a microservices architecture.

## 2. Architectural Blueprint
- **Communication:** RESTful APIs for external traffic, gRPC for internal service-to-service communication.
- **Database:** "Database per Service" pattern using PostgreSQL with isolated schemas.
- **Security:** Centralized JWT-based Identity Service.
- **Scalability:** Dockerized services, ready for Kubernetes (K8s) orchestration.

## 3. Service Breakdown

### A. Identity Service (`/apps/identity-svc`)
- **Responsibility:** Authentication, Authorization, User Profile Management.
- **Tech Stack:** Go (Golang), PostgreSQL, JWT.
- **Key Features:**
  - User Registration & Login.
  - Role-Based Access Control (RBAC) - [Admin, Scout, User].
  - Token Validation Middleware for other services.

### B. Football Service (`/apps/football-svc`)
- **Responsibility:** Core football domain logic.
- **Tech Stack:** Go (Golang), PostgreSQL, gRPC.
- **Key Features:**
  - League & Competition management.
  - Team & Club tracking.
  - Player profiles & Transfer history.
  - Coach/Staff records.

### C. Frontend Dashboard (`/apps/frontend`)
- **Responsibility:** User interface and data visualization.
- **Tech Stack:** Angular, Vanilla CSS, RxJS.
- **Key Features:**
  - Modern, responsive dashboard.
  - Real-time data updates via REST/WebSockets.
  - Admin panel for data entry.

## 4. Infrastructure & DevOps
- **Containerization:** Docker & Docker Compose.
- **CI/CD:** GitHub Actions (Automated Build/Test).
- **Environment:** Development, Staging, Production.

## 6. Communication & Education Protocol
- **End-of-Step Summaries:** At the conclusion of every technical step, a detailed explanation must be provided.
- **Target Audience:** Junior Developer.
- **Assumed Knowledge:** JavaScript, Node.js (Express/NestJS), and React.
- **Constraint:** Assume NO prior experience with Go (Golang) or Angular.
- **Strict Command Protocol:** The agent must only act on direct user commands. No improvised steps or unsolicited architectural changes are permitted.
- **Progress Tracking:** Every completed command or decision must be recorded in this specification file.

## 5. Progress Tracker

### Phase 1: Foundation & Infrastructure 🏗️
- [x] 1.1 Repository & Workspace Initialization
- [x] 1.2 Containerization Strategy (Docker & CI/CD)
- [x] 1.3 Foundation Specs (PROJECT_SPEC.md)

### Phase 2: Identity Service (Auth) 🔐
- [x] 2.1 Service Initialization (Go Mod, Postgres Setup)
- [x] 2.2 User Model & Migrations
- [x] 2.3 JWT Implementation & Auth Routes
- [x] 2.4 Username Support & Shared Auth Library

### Phase 3: Football Service (Domain) ⚽
- [x] 3.1 Service Initialization
- [ ] 3.2 Core Models (Leagues, Teams, Players)
- [ ] 3.3 Internal gRPC Communication
- [x] 3.4 Auth Middleware Integration

### Phase 4: Frontend Development (UI) 🎨
- [ ] 4.1 Angular Workspace Setup
- [ ] 4.2 State Management & Auth Integration
- [ ] 4.3 Domain Components (Player Cards, League Tables)
