package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/haidarrasyad/absensi-api/internal/pkg/apperror"
	"github.com/haidarrasyad/absensi-api/internal/pkg/httpx"
	"github.com/haidarrasyad/absensi-api/internal/pkg/token"
)

type claimsKey struct{}

// Guards berisi middleware untuk route yang wajib login.
type Guards struct {
	Employee Middleware
	Admin    Middleware
	Upload   Middleware
}

// NewGuards membuat guard pegawai dan admin.
func NewGuards(tokens *token.Manager) Guards {
	return Guards{
		Employee: requireRole(tokens, token.RoleEmployee),
		Admin:    requireRole(tokens, token.RoleAdmin),
		Upload:   func(next http.Handler) http.Handler { return next },
	}
}

func requireRole(tokens *token.Manager, role token.Role) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := ""
			if authorization := r.Header.Get("Authorization"); authorization != "" {
				parts := strings.Fields(authorization)
				if len(parts) != 2 || parts[0] != "Bearer" {
					httpx.Fail(w, r, apperror.Unauthorized("unauthorized", "Authorization tidak valid."))
					return
				}
				raw = parts[1]
			}
			if raw == "" {
				if cookie, err := r.Cookie("access_token"); err == nil {
					raw = cookie.Value
				}
			}
			claims, err := tokens.Parse(r.Context(), raw)
			if err != nil || claims.Role != role {
				httpx.Fail(w, r, apperror.Unauthorized("unauthorized", "Sesi berakhir. Silakan masuk lagi."))
				return
			}
			ctx := context.WithValue(r.Context(), claimsKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SubjectFrom mengambil ID pengguna yang sedang login (dipasang oleh guard).
func SubjectFrom(ctx context.Context) string {
	claims, ok := ctx.Value(claimsKey{}).(*token.Claims)
	if !ok {
		return ""
	}
	return claims.Subject
}
