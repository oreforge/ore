package dto

type StreamLine struct {
	Time  string `json:"time,omitempty" example:"2026-01-01T00:00:00.000Z"`
	Level string `json:"level,omitempty" example:"INFO"`
	Msg   string `json:"msg,omitempty" example:"building image"`
	Done  bool   `json:"done,omitempty" example:"true"`
	Error string `json:"error,omitempty" example:"build failed"`
}
