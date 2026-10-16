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

// SignupResponse is deliberately value-identical for every accepted
// registration attempt (review CR-03): a populated internal identifier used
// to distinguish a fresh account from a duplicate/reserved address, which is
// an unauthenticated existence oracle. Ownership is established by logging
// in; /api/me then reports the id.
type SignupResponse struct {
	OK bool `json:"ok"`
}
