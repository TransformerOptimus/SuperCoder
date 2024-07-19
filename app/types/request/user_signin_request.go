package request

type UserSignInRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
