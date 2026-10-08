package auth

type SigninRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type SignupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}
type SignupResponse struct {
	OK     bool   `json:"ok"`
	UserID string `json:"userId"`
}
