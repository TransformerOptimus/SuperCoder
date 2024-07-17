package request

type CreateUserRequest struct {
	Email          string `json:"email"`
	Password       string `json:"password"`
	OrganisationID *uint  `json:"organisation_id,omitempty"`
}
