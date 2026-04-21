package dto

type BatchTargetsRequest struct {
	Targets []string `json:"targets" validate:"required,min=1,max=100,dive,required"`
}

type BatchVolumeDeleteRequest struct {
	Targets []string `json:"targets" validate:"required,min=1,max=100,dive,required"`
	Force   bool     `json:"force,omitempty"`
}

type BatchTargetResult struct {
	Target string `json:"target"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type BatchResponse struct {
	OperationID string              `json:"operation_id"`
	Status      string              `json:"status"`
	Total       int                 `json:"total"`
	Succeeded   int                 `json:"succeeded"`
	Failed      int                 `json:"failed"`
	Skipped     int                 `json:"skipped"`
	Results     []BatchTargetResult `json:"results"`
}
