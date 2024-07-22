package response

type UserResponse struct {
	Id             uint   `json:"id"`
	Email          string `json:"email"`
	Name           string `json:"name"`
	OrganisationID uint   `json:"organisation_id"`
}
