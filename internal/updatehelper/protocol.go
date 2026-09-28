package updatehelper

type Request struct {
	Operation     string `json:"operation"`
	PackagePath   string `json:"package_path"`
	SHA256        string `json:"sha256"`
	Version       string `json:"version"`
	DebianVersion string `json:"debian_version"`
	Architecture  string `json:"architecture"`
}

type Response struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}
