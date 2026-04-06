package dto

type AddProjectRequest struct {
	URL  string `json:"url" validate:"required" example:"https://github.com/org/network.git"`
	Name string `json:"name,omitempty" example:"mynetwork"`
}

type ProjectResponse struct {
	Name string `json:"name" example:"mynetwork"`
}

type ProjectListResponse struct {
	Projects []string `json:"projects"`
}

type WebhookInfoResponse struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url,omitempty" example:"/webhook/mynetwork?secret=abc123"`
	Secret  string `json:"secret,omitempty"`
	Force   bool   `json:"force,omitempty"`
	NoCache bool   `json:"no_cache,omitempty"`
}

type WebhookResponse struct {
	Status  string `json:"status" example:"accepted"`
	Project string `json:"project" example:"mynetwork"`
}
