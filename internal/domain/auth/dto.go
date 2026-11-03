package auth

import (
	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
	"strings"
	"time"
)

type EmployeeLoginRequest struct {
	Fullname string `json:"fullname" validate:"required,notblank,max=80" label:"Nama Lengkap"`
	NIK      string `json:"nik" validate:"required,nik" label:"NIK"`
}
type AdminLoginRequest struct {
	Email    string `json:"email" validate:"required,notblank,max=254,email" label:"Email"`
	Password string `json:"password" validate:"required,notblank,maxbytes=72" label:"Password"`
}
type SessionResponse struct {
	Token        string              `json:"-"`
	RefreshToken string              `json:"-"`
	ExpiresAt    time.Time           `json:"expiresAt"`
	Employee     *employees.Response `json:"employee,omitempty"`
}

// Normalize cleans identifiers while preserving password bytes exactly.
func (r *AdminLoginRequest) Normalize() { r.Email = strings.TrimSpace(r.Email) }
func (r *EmployeeLoginRequest) Normalize() {
	r.Fullname = strings.Join(strings.Fields(r.Fullname), " ")
	r.NIK = strings.TrimSpace(r.NIK)
}
