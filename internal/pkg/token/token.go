// Package token manages signed access tokens and revocable database sessions.
package token

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Role string

const (
	RoleEmployee Role = "karyawan"
	RoleAdmin    Role = "admin"
)

// RevocationTimeout includes waiting for a pool connection and executing DELETE.
const RevocationTimeout = 10 * time.Second

var ErrRevocationTimeout = errors.New("session revocation timed out")

type Claims struct {
	Role Role `json:"role"`
	jwt.RegisteredClaims
}
type Manager struct {
	secret []byte
	db     *pgxpool.Pool
}

func NewManager(secret string, db *pgxpool.Pool) *Manager { return &Manager{[]byte(secret), db} }
func hash(raw string) string                              { sum := sha256.Sum256([]byte(raw)); return hex.EncodeToString(sum[:]) }
func randomToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}
func (m *Manager) sign(id, subject string, role Role) (string, time.Time, error) {
	now := time.Now()
	expiry := now.Add(15 * time.Minute)
	c := Claims{role, jwt.RegisteredClaims{ID: id, Subject: subject, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expiry)}}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(m.secret)
	return raw, expiry, err
}
func (m *Manager) Issue(ctx context.Context, subject string, role Role) (string, string, time.Time, error) {
	refresh, err := randomToken()
	if err != nil {
		return "", "", time.Time{}, err
	}
	ttl := 7 * 24 * time.Hour
	if role == RoleEmployee {
		ttl = 90 * 24 * time.Hour
	}
	var id string
	var userID, employeeID any
	switch role {
	case RoleAdmin:
		userID = subject
	case RoleEmployee:
		employeeID = subject
	default:
		return "", "", time.Time{}, fmt.Errorf("invalid session role")
	}
	err = m.db.QueryRow(ctx, `INSERT INTO sessions(user_id,employee_id,role,refresh_hash,expires_at) VALUES($1,$2,$3,$4,$5) RETURNING session_id`, userID, employeeID, role, hash(refresh), time.Now().Add(ttl)).Scan(&id)
	if err != nil {
		return "", "", time.Time{}, err
	}
	access, expiry, err := m.sign(id, subject, role)
	return access, refresh, expiry, err
}

const activeSession = ` AND ((role='admin' AND EXISTS(SELECT 1 FROM users WHERE users.user_id=sessions.user_id AND active AND users.role='admin')) OR (role='karyawan' AND EXISTS(SELECT 1 FROM employees WHERE employees.employee_id=sessions.employee_id AND is_active)))`

func (m *Manager) Parse(ctx context.Context, raw string) (*Claims, error) {
	c := &Claims{}
	_, err := jwt.ParseWithClaims(raw, c, func(*jwt.Token) (any, error) { return m.secret, nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	var valid bool
	err = m.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sessions WHERE session_id=$1 AND COALESCE(user_id,employee_id)=$2 AND role=$3 AND expires_at>now()`+activeSession+`)`, c.ID, c.Subject, c.Role).Scan(&valid)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, fmt.Errorf("session expired")
	}
	return c, nil
}
func (m *Manager) Refresh(ctx context.Context, raw string, role Role) (string, string, time.Time, error) {
	next, err := randomToken()
	if err != nil {
		return "", "", time.Time{}, err
	}
	var id, subject string
	// Atomic rotation: an old refresh token can only be used once.
	err = m.db.QueryRow(ctx, `UPDATE sessions SET refresh_hash=$1 WHERE refresh_hash=$2 AND role=$3 AND expires_at>now()`+activeSession+` RETURNING session_id,COALESCE(user_id,employee_id)`, hash(next), hash(raw), role).Scan(&id, &subject)
	if err != nil {
		return "", "", time.Time{}, err
	}
	access, expiry, err := m.sign(id, subject, role)
	return access, next, expiry, err
}

// Logout can identify the same session through its signed access cookie even if
// another request just rotated the refresh token. Expiry is deliberately ignored
// only for revocation; expired access tokens still cannot authorize API requests.
func (m *Manager) Revoke(ctx context.Context, raw string, role Role, access ...string) error {
	ctx, cancel := context.WithTimeout(ctx, RevocationTimeout)
	defer cancel()
	var id, subject string
	if len(access) > 0 && access[0] != "" {
		c := &Claims{}
		if _, err := jwt.ParseWithClaims(access[0], c, func(*jwt.Token) (any, error) { return m.secret, nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithoutClaimsValidation()); err == nil && c.Role == role {
			id, subject = c.ID, c.Subject
		}
	}
	_, err := m.db.Exec(ctx, `DELETE FROM sessions WHERE role=$2 AND (refresh_hash=$1 OR (session_id=$3 AND COALESCE(user_id,employee_id)=$4))`, hash(raw), role, id, subject)
	if err != nil && (errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded)) {
		return fmt.Errorf("%w: %v", ErrRevocationTimeout, err)
	}
	return err
}
