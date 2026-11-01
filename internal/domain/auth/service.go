package auth

import (
	"context"
	"errors"
	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
	"github.com/haidarrasyad/absensi-api/internal/pkg/token"
	"github.com/haidarrasyad/absensi-api/internal/shared"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"strings"
	"time"
	"unicode/utf8"
)

type EmployeeFinder interface {
	GetByID(context.Context, string) (employees.Employee, error)
	GetByName(context.Context, string) (employees.Employee, error)
}

// SessionManager describes the session operations used by authentication.
type SessionManager interface {
	Issue(context.Context, string, token.Role) (string, string, time.Time, error)
	Refresh(context.Context, string, token.Role) (string, string, time.Time, error)
	Revoke(context.Context, string, token.Role, ...string) error
}
type Service struct {
	employees        EmployeeFinder
	repo             *Repository
	tokens           SessionManager
	limiter          *shared.AttemptLimiter
	adminFailures    *shared.AttemptLimiter
	hasher           shared.PhoneHasher
	employeeRequests *shared.AttemptLimiter
	adminRequests    *shared.AttemptLimiter
}

func NewService(e EmployeeFinder, r *Repository, t SessionManager, hasher shared.PhoneHasher, employeeLoginPerIP int) *Service {
	return &Service{employees: e, repo: r, tokens: t, limiter: shared.NewAttemptLimiter(5, 15*time.Minute), adminFailures: shared.NewAttemptLimiter(10, 15*time.Minute), hasher: hasher, employeeRequests: shared.NewAttemptLimiter(employeeLoginPerIP, time.Minute), adminRequests: shared.NewAttemptLimiter(100, time.Minute)}
}
func (s *Service) LoginEmployee(ctx context.Context, ip string, req EmployeeLoginRequest) (SessionResponse, error) {
	if len(req.Fullname) > 320 || utf8.RuneCountInString(req.Fullname) > 80 || len(req.Phone) > 32 || req.Phone == "" {
		return SessionResponse{}, ErrInvalidCredentials
	}
	key := "employee:" + ip + ":" + strings.ToLower(strings.Join(strings.Fields(req.Fullname), " "))
	if !s.employeeRequests.Allow("login:"+ip) || !s.limiter.Allow(key) {
		return SessionResponse{}, ErrTooManyAttempts
	}
	e, err := s.employees.GetByName(ctx, req.Fullname)
	if err != nil && !errors.Is(err, employees.ErrNotFound) {
		return SessionResponse{}, err
	}
	if err != nil || !e.IsActive || !s.hasher.Matches(req.Phone, e.PhoneHash) {
		return SessionResponse{}, ErrInvalidCredentials
	}
	s.limiter.Reset(key)
	result, err := s.issue(ctx, e.EmployeeID, token.RoleEmployee)
	profile := employees.ToResponse(e)
	result.Employee = &profile
	return result, err
}
func (s *Service) LoginAdmin(ctx context.Context, ip string, req AdminLoginRequest) (SessionResponse, error) {
	if len(req.Email) > 254 || len(req.Password) > 72 {
		return SessionResponse{}, ErrInvalidCredentials
	}
	key := "admin:" + ip + ":" + strings.ToLower(strings.TrimSpace(req.Email))
	if !s.adminRequests.Allow("login:"+ip) || !s.adminFailures.Allow(key) {
		return SessionResponse{}, ErrTooManyAttempts
	}
	a, err := s.repo.FindAdmin(ctx, strings.TrimSpace(req.Email))
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return SessionResponse{}, err
	}
	hash := a.PasswordHash
	if err != nil {
		hash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
	}
	passwordErr := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password))
	if err != nil || passwordErr != nil {
		return SessionResponse{}, ErrInvalidCredentials
	}
	s.adminFailures.Reset(key)
	return s.issue(ctx, a.UserID, token.RoleAdmin)
}
func (s *Service) issue(ctx context.Context, id string, role token.Role) (SessionResponse, error) {
	access, refresh, expiry, err := s.tokens.Issue(ctx, id, role)
	return SessionResponse{Token: access, RefreshToken: refresh, ExpiresAt: expiry}, err
}
func (s *Service) CurrentEmployee(ctx context.Context, id string) (employees.Response, error) {
	e, err := s.employees.GetByID(ctx, id)
	return employees.ToResponse(e), err
}
