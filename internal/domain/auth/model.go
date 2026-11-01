package auth

type Admin struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	Fullname     string `json:"fullname"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	PasswordHash string `json:"-"`
}
