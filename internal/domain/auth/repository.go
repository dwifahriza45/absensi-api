package auth

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }
func (r *Repository) FindAdmin(ctx context.Context, email string) (Admin, error) {
	var a Admin
	err := r.db.QueryRow(ctx, `SELECT id::text, user_id, fullname, email, role, password_hash FROM users WHERE lower(email)=lower($1) AND active AND role='admin'`, email).Scan(&a.ID, &a.UserID, &a.Fullname, &a.Email, &a.Role, &a.PasswordHash)
	return a, err
}
func (r *Repository) AdminByID(ctx context.Context, id string) (Admin, error) {
	var a Admin
	err := r.db.QueryRow(ctx, `SELECT id::text, user_id, fullname, email, role FROM users WHERE user_id=$1 AND active AND role='admin'`, id).Scan(&a.ID, &a.UserID, &a.Fullname, &a.Email, &a.Role)
	return a, err
}

// RejectDefaultPassword prevents a seeded demonstration credential from going live.
func (r *Repository) RejectDefaultPassword(ctx context.Context) error {
	rows, err := r.db.Query(ctx, `SELECT password_hash FROM users WHERE active AND role='admin'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var hash string
		if err = rows.Scan(&hash); err != nil {
			return err
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte("Admin123!")) == nil {
			return errors.New("akun admin masih memakai password bawaan; ganti sebelum menjalankan production")
		}
	}
	return rows.Err()
}
