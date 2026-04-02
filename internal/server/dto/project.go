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

type UpdateProjectResponse struct {
	Name   string `json:"name" example:"mynetwork"`
	Status string `json:"status" example:"deployed"`
}
