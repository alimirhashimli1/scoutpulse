# Identity Service

The Identity Service is responsible for user authentication, authorization, and profile management in the ScoutPulse Micro ecosystem.

## Features

- **User Registration**: Create a new account with a username, email, password, and role.
- **User Login**: Authenticate with email/username and password to receive a JWT.
- **RBAC**: Role-Based Access Control support (ADMIN, EDITOR, USER).

## API Endpoints

### Register User
`POST /register`

Registers a new user in the system.

**Request Body:**
```json
{
  "username": "johndoe",
  "email": "john@example.com",
  "password": "securepassword123",
  "role": "USER"
}
```

**Response (201 Created):**
```json
{
  "id": "uuid-v4-string",
  "username": "johndoe",
  "email": "john@example.com",
  "role": "USER"
}
```

### Login User
`POST /login`

Authenticates a user and returns a JWT token.

**Request Body:**
```json
{
  "identifier": "johndoe", 
  "password": "securepassword123"
}
```
*Note: `identifier` can be either the username or the email.*

**Response (200 OK):**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

## Tech Stack

- **Language**: Go (Golang)
- **Database**: PostgreSQL
- **Auth**: JWT (JSON Web Tokens)
- **Libraries**:
    - `github.com/golang-jwt/jwt/v5`
    - `golang.org/x/crypto/bcrypt`
    - `github.com/jmoiron/sqlx`
    - `github.com/lib/pq`

## Database Schema

The service uses a `users` table:
- `id`: UUID (Primary Key)
- `username`: TEXT (Unique)
- `email`: TEXT (Unique)
- `password_hash`: TEXT
- `role`: TEXT
