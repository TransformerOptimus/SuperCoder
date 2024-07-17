package request

type InviteUserRequest struct {
	Email         string `json:"email"`
	CurrentUserID int    `json:"current_user_id"`
}
