package updaterhelper

type Request struct {
	Operation  string `json:"operation"`
	Version    string `json:"version,omitempty"`
	Device     string `json:"device,omitempty"`
	Mountpoint string `json:"mountpoint,omitempty"`
	Filesystem string `json:"filesystem,omitempty"`
	Label      string `json:"label,omitempty"`
	Confirm    string `json:"confirm,omitempty"`
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
