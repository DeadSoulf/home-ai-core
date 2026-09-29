package updaterhelper

const ProtocolVersion = 2

// HelperVersion is injected at build time. "dev" is used for local builds.
var HelperVersion = "dev"

type Request struct {
	Operation       string `json:"operation"`
	ProtocolVersion int    `json:"protocol_version,omitempty"`
	Version         string `json:"version,omitempty"`
	Device          string `json:"device,omitempty"`
	Mountpoint      string `json:"mountpoint,omitempty"`
	Filesystem      string `json:"filesystem,omitempty"`
	Label           string `json:"label,omitempty"`
	Confirm         string `json:"confirm,omitempty"`
	SizeMiB         uint64 `json:"size_mib,omitempty"`
}

type Response struct {
	OK              bool   `json:"ok"`
	Message         string `json:"message,omitempty"`
	Error           string `json:"error,omitempty"`
	HelperVersion   string `json:"helper_version,omitempty"`
	ProtocolVersion int    `json:"protocol_version,omitempty"`
}

type Result struct {
	Status    string `json:"status"`
	Version   string `json:"version,omitempty"`
	Message   string `json:"message,omitempty"`
	Error     string `json:"error,omitempty"`
	UpdatedAt string `json:"updated_at"`
}
