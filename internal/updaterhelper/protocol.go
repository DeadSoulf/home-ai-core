package updaterhelper

const ProtocolVersion = 2

// HelperVersion is injected at build time. "dev" is used for local builds.
var HelperVersion = "dev"

type Request struct {
	Operation       string `json:"operation"`
	ProtocolVersion int    `json:"protocol_version,omitempty"`
	Version         string `json:"version,omitempty"`
	CurrentVersion  string `json:"current_version,omitempty"`
	Device          string `json:"device,omitempty"`
	Mountpoint      string `json:"mountpoint,omitempty"`
	Filesystem      string `json:"filesystem,omitempty"`
	Label           string `json:"label,omitempty"`
	Confirm         string `json:"confirm,omitempty"`
	SizeMiB         uint64 `json:"size_mib,omitempty"`
}

type FilesystemStat struct {
	Device     string `json:"device"`
	Filesystem string `json:"filesystem,omitempty"`
	FreeBytes  uint64 `json:"free_bytes,omitempty"`
	FreeKnown  bool   `json:"free_known"`
}

type DiskHealthStat struct {
	Device           string  `json:"device"`
	Transport        string  `json:"transport,omitempty"`
	Health           string  `json:"health,omitempty"`
	TemperatureC     *int    `json:"temperature_c,omitempty"`
	PowerOnHours     *uint64 `json:"power_on_hours,omitempty"`
	LifeRemainingPct *int    `json:"life_remaining_percent,omitempty"`
	SmartAvailable   bool    `json:"smart_available"`
	SmartError       string  `json:"smart_error,omitempty"`
}

type LVMStat struct {
	Device          string  `json:"device,omitempty"`
	Name            string  `json:"name"`
	VGName          string  `json:"vg_name"`
	LVName          string  `json:"lv_name"`
	SizeBytes       uint64  `json:"size_bytes,omitempty"`
	Active          bool    `json:"active"`
	DataPercent     *float64 `json:"data_percent,omitempty"`
	MetadataPercent *float64 `json:"metadata_percent,omitempty"`
}

type Response struct {
	OK              bool             `json:"ok"`
	Message         string           `json:"message,omitempty"`
	Error           string           `json:"error,omitempty"`
	HelperVersion   string           `json:"helper_version,omitempty"`
	ProtocolVersion  int              `json:"protocol_version,omitempty"`
	RollbackAvailable bool             `json:"rollback_available,omitempty"`
	RollbackVersion   string           `json:"rollback_version,omitempty"`
	FilesystemStats  []FilesystemStat `json:"filesystem_stats,omitempty"`
	DiskHealth       []DiskHealthStat  `json:"disk_health,omitempty"`
	LVM              []LVMStat         `json:"lvm,omitempty"`
}

type Result struct {
	Status    string `json:"status"`
	Version   string `json:"version,omitempty"`
	Message   string `json:"message,omitempty"`
	Error     string `json:"error,omitempty"`
	UpdatedAt string `json:"updated_at"`
}
