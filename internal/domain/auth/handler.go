package auth

import (
	"errors"
	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
	"github.com/haidarrasyad/absensi-api/internal/pkg/apperror"
	"github.com/haidarrasyad/absensi-api/internal/pkg/httpx"
	"github.com/haidarrasyad/absensi-api/internal/pkg/token"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

const loginBodyLimit = 4 << 10 // Login hanya memerlukan field pendek.

type Handler struct {
	svc    *Service
	secure bool
}

func NewHandler(s *Service, secure bool) *Handler { return &Handler{s, secure} }

func (h *Handler) cookies(w http.ResponseWriter, s SessionResponse, role token.Role) {
	ttl := 7 * 24 * time.Hour
	if role == token.RoleEmployee {
		ttl = 90 * 24 * time.Hour
	}
	for name, value := range map[string]string{"access_token": s.Token, "refresh_token": s.RefreshToken} {
		age := int(time.Until(s.ExpiresAt).Seconds())
		if name == "refresh_token" {
			age = int(ttl.Seconds())
		}
		http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/api/v1/" + string(role), MaxAge: age, HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode})
	}
}
func (h *Handler) LoginEmployee(w http.ResponseWriter, r *http.Request) {
	var req EmployeeLoginRequest
	if err := httpx.DecodeJSONLimit(w, r, &req, loginBodyLimit); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s, err := h.svc.LoginEmployee(r.Context(), httpx.ClientIP(r), req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.cookies(w, s, token.RoleEmployee)
	httpx.OK(w, 200, s, "Login Successfully")
}
func (h *Handler) LoginAdmin(w http.ResponseWriter, r *http.Request) {
	var req AdminLoginRequest
	if err := httpx.DecodeJSONLimit(w, r, &req, loginBodyLimit); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s, err := h.svc.LoginAdmin(r.Context(), httpx.ClientIP(r), req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.cookies(w, s, token.RoleAdmin)
	httpx.OK(w, 200, s, "Login Successfully")
}
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.CurrentEmployee(r.Context(), middleware.SubjectFrom(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, 200, p, "User Retrieved Successfully")
}
func (h *Handler) AdminMe(w http.ResponseWriter, r *http.Request) {
	a, err := h.svc.repo.AdminByID(r.Context(), middleware.SubjectFrom(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, 200, a, "User Retrieved Successfully")
}
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request, role token.Role) {
	c, err := r.Cookie("refresh_token")
	if err != nil {
		httpx.Fail(w, r, ErrInvalidRefreshToken)
		return
	}
	a, refresh, expiry, err := h.svc.tokens.Refresh(r.Context(), c.Value, role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Fail(w, r, ErrInvalidRefreshToken)
		} else {
			httpx.Fail(w, r, err)
		}
		return
	}
	s := SessionResponse{Token: a, RefreshToken: refresh, ExpiresAt: expiry}
	h.cookies(w, s, role)
	httpx.OK(w, 200, s, "Token Refreshed Successfully")
}
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request, role token.Role) {
	var refresh, access string
	if c, err := r.Cookie("refresh_token"); err == nil {
		refresh = c.Value
	}
	if c, err := r.Cookie("access_token"); err == nil {
		access = c.Value
	}
	if err := h.svc.tokens.Revoke(r.Context(), refresh, role, access); err != nil {
		if errors.Is(err, token.ErrRevocationTimeout) {
			w.Header().Set("Retry-After", "2")
			httpx.Fail(w, r, apperror.New(http.StatusGatewayTimeout, "logout_timeout", "Batas waktu logout tercapai. Silakan coba lagi."))
			return
		}
		httpx.Fail(w, r, err)
		return
	}

	for _, name := range []string{"access_token", "refresh_token"} {
		http.SetCookie(w, &http.Cookie{Name: name, Path: "/api/v1/" + string(role), MaxAge: -1, HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode})
	}
	httpx.OK(w, 200, nil, "Logout Successfully")
}
