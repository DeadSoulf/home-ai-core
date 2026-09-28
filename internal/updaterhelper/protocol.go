package updaterhelper

type Request struct {
	Operation string `json:"operation"`
	Version   string `json:"version"`
}

type Response struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type Result struct {
	Status    string `json:"status"`
	Version   string `json:"version,omitempty"`
	Message   string `json:"message,omitempty"`
	Error     string `json:"error,omitempty"`
	UpdatedAt string `json:"updated_at"`
}
