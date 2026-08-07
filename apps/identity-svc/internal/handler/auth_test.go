package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"os"

	"github.com/scoutpulse/identity-svc/internal/model"
	"github.com/scoutpulse/libs/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// TestMain installs a signing key. Token operations fail closed without one,
// so every test in this package needs it.
func TestMain(m *testing.M) {
	if err := auth.SetSecret([]byte("identity-svc-test-signing-key-long-enough")); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// MockUserRepository is a mock type for the UserRepository interface
type MockUserRepository struct {
	mock.Mock
}

func (m *MockUserRepository) Create(ctx context.Context, user *model.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockUserRepository) GetByIdentifier(ctx context.Context, identifier string) (*model.User, error) {
	args := m.Called(ctx, identifier)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.User), args.Error(1)
}

func TestHealth(t *testing.T) {
	h := &Handler{}
	req, err := http.NewRequest("GET", "/health", nil)
	assert.NoError(t, err)

	rr := httptest.NewRecorder()
	h.Health(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "Identity Service is healthy", rr.Body.String())
}

func TestRegister(t *testing.T) {
	mockRepo := new(MockUserRepository)
	h := &Handler{UserRepo: mockRepo}

	regReq := RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "password123",
	}
	body, _ := json.Marshal(regReq)

	// Expect Create to be called with any user object
	// We'll verify the password hashing logic here
	mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*model.User")).Return(nil).Run(func(args mock.Arguments) {
		user := args.Get(1).(*model.User)
		assert.Equal(t, regReq.Username, user.Username)
		assert.Equal(t, regReq.Email, user.Email)
		// Verify password is NOT plain text
		assert.NotEqual(t, regReq.Password, user.PasswordHash)
		// Verify it's a valid bcrypt hash
		err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(regReq.Password))
		assert.NoError(t, err)
	})

	req, _ := http.NewRequest("POST", "/api/v1/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Register(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	mockRepo.AssertExpectations(t)
}

// TestRegister_IgnoresClientSuppliedRole is a regression test for the
// privilege-escalation hole where Register trusted a "role" field from the
// request body, letting anyone create an admin account.
func TestRegister_IgnoresClientSuppliedRole(t *testing.T) {
	mockRepo := new(MockUserRepository)
	h := &Handler{UserRepo: mockRepo}

	var created *model.User
	mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*model.User")).
		Return(nil).
		Run(func(args mock.Arguments) {
			created = args.Get(1).(*model.User)
		})

	// A hand-crafted body carrying the field the struct no longer declares.
	body := []byte(`{"username":"attacker","email":"a@example.com","password":"password123","role":"admin"}`)

	req, _ := http.NewRequest("POST", "/api/v1/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Register(rr, req)

	// DecodeJSON rejects unknown fields, so the request is refused outright.
	// Even if that guard were relaxed, the assertion below must hold.
	if rr.Code == http.StatusCreated {
		require.NotNil(t, created)
		assert.Equal(t, model.UserRole, created.Role,
			"self-registration must never assign a privileged role")
	} else {
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	}
}

func TestRegister_RejectsShortPassword(t *testing.T) {
	mockRepo := new(MockUserRepository)
	h := &Handler{UserRepo: mockRepo}

	body, _ := json.Marshal(RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "short",
	})

	req, _ := http.NewRequest("POST", "/api/v1/auth/register", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	h.Register(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	mockRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestLogin(t *testing.T) {
	password := "password123"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	testUser := &model.User{
		ID:           "test-id",
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: string(hashedPassword),
		Role:         model.UserRole,
	}

	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockUserRepository)
		h := &Handler{UserRepo: mockRepo}

		loginReq := LoginRequest{
			Identifier: "test@example.com",
			Password:   password,
		}
		body, _ := json.Marshal(loginReq)

		mockRepo.On("GetByIdentifier", mock.Anything, loginReq.Identifier).Return(testUser, nil)

		req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		h.Login(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]string
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.NotEmpty(t, resp["token"])
		mockRepo.AssertExpectations(t)
	})

	t.Run("Invalid Password", func(t *testing.T) {
		mockRepo := new(MockUserRepository)
		h := &Handler{UserRepo: mockRepo}

		loginReq := LoginRequest{
			Identifier: "test@example.com",
			Password:   "wrongpassword",
		}
		body, _ := json.Marshal(loginReq)

		mockRepo.On("GetByIdentifier", mock.Anything, loginReq.Identifier).Return(testUser, nil)

		req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		h.Login(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		mockRepo.AssertExpectations(t)
	})

	t.Run("User Not Found", func(t *testing.T) {
		mockRepo := new(MockUserRepository)
		h := &Handler{UserRepo: mockRepo}

		loginReq := LoginRequest{
			Identifier: "nonexistent@example.com",
			Password:   password,
		}
		body, _ := json.Marshal(loginReq)

		mockRepo.On("GetByIdentifier", mock.Anything, loginReq.Identifier).Return(nil, errors.New("not found"))

		req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		h.Login(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		mockRepo.AssertExpectations(t)
	})
}
