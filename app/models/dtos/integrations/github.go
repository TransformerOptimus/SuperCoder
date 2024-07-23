package integrations

type GithubIntegrationDetails struct {
	UserId       uint64
	GithubUserId string
	AccessToken  string
	RefreshToken *string
}
