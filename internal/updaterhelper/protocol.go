package updaterhelper

const ProtocolVersion = 7

// HelperVersion is injected at build time. "dev" is used for local builds.
var HelperVersion = "dev"

type Request struct {
	Operation         string            `json:"operation"`
	ProtocolVersion   int               `json:"protocol_version,omitempty"`
	Version           string            `json:"version,omitempty"`
	CurrentVersion    string            `json:"current_version,omitempty"`
	Device            string            `json:"device,omitempty"`
	Mountpoint        string            `json:"mountpoint,omitempty"`
	Filesystem        string            `json:"filesystem,omitempty"`
	Label             string            `json:"label,omitempty"`
	Confirm           string            `json:"confirm,omitempty"`
	DryRun            bool              `json:"dry_run,omitempty"`
	SizeMiB           uint64            `json:"size_mib,omitempty"`
	Interface         string            `json:"interface,omitempty"`
	Address           string            `json:"address,omitempty"`
	Gateway           string            `json:"gateway,omitempty"`
	MTU               int               `json:"mtu,omitempty"`
	Tunnel            string            `json:"tunnel,omitempty"`
	ListenPort        int               `json:"listen_port,omitempty"`
	PrivateKey        string            `json:"private_key,omitempty"`
	PeerPublicKey     string            `json:"peer_public_key,omitempty"`
	PresharedKey      string            `json:"preshared_key,omitempty"`
	AllowedIPs        []string          `json:"allowed_ips,omitempty"`
	Endpoint          string            `json:"endpoint,omitempty"`
	Keepalive         int               `json:"keepalive,omitempty"`
	NetworkMethod     string            `json:"network_method,omitempty"`
	DNS               []string          `json:"dns,omitempty"`
	RootPath          string            `json:"root_path,omitempty"`
	ProjectID         uint32            `json:"project_id,omitempty"`
	QuotaBytes        int64             `json:"quota_bytes,omitempty"`
	RelativePath      string            `json:"relative_path,omitempty"`
	ReservePercent    int               `json:"reserve_percent,omitempty"`
	SMBWorkgroup      string            `json:"smb_workgroup,omitempty"`
	SMBUser           string            `json:"smb_user,omitempty"`
	SMBPassword       string            `json:"smb_password,omitempty"`
	SMBUsers          []string          `json:"smb_users,omitempty"`
	SMBShares         []SMBShareRequest `json:"smb_shares,omitempty"`
	ModuleID          string            `json:"module_id,omitempty"`
	Image             string            `json:"image,omitempty"`
	ModuleHealthPort  int               `json:"module_health_port,omitempty"`
	ModuleHealthPath  string            `json:"module_health_path,omitempty"`
	RegistryUsername  string            `json:"registry_username,omitempty"`
	RegistryToken     string            `json:"registry_token,omitempty"`
}

type SMBShareRequest struct {
	Name           string   `json:"name"`
	Path           string   `json:"path"`
	PoolRoot       string   `json:"pool_root,omitempty"`
	ReservePercent int      `json:"reserve_percent,omitempty"`
	ReadUsers      []string `json:"read_users,omitempty"`
	WriteUsers     []string `json:"write_users,omitempty"`
}

type FilesystemStat struct {
	Device     string `json:"device"`
	Filesystem string `json:"filesystem,omitempty"`
	Mountpoint string `json:"mountpoint,omitempty"`
	TotalBytes uint64 `json:"total_bytes,omitempty"`
	TotalKnown bool   `json:"total_known"`
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
	Device          string   `json:"device,omitempty"`
	Name            string   `json:"name"`
	VGName          string   `json:"vg_name"`
	LVName          string   `json:"lv_name"`
	SizeBytes       uint64   `json:"size_bytes,omitempty"`
	VGSizeBytes     uint64   `json:"vg_size_bytes,omitempty"`
	VGFreeBytes     uint64   `json:"vg_free_bytes,omitempty"`
	Active          bool     `json:"active"`
	DataPercent     *float64 `json:"data_percent,omitempty"`
	MetadataPercent *float64 `json:"metadata_percent,omitempty"`
}

type WireGuardPeerStat struct {
	PublicKey       string   `json:"public_key"`
	Endpoint        string   `json:"endpoint,omitempty"`
	AllowedIPs      []string `json:"allowed_ips,omitempty"`
	LatestHandshake int64    `json:"latest_handshake,omitempty"`
	TransferRX      uint64   `json:"transfer_rx,omitempty"`
	TransferTX      uint64   `json:"transfer_tx,omitempty"`
	Keepalive       int      `json:"keepalive,omitempty"`
}

type WireGuardTunnelStat struct {
	Name       string              `json:"name"`
	Active     bool                `json:"active"`
	Address    string              `json:"address,omitempty"`
	PublicKey  string              `json:"public_key,omitempty"`
	ListenPort int                 `json:"listen_port,omitempty"`
	Peers      []WireGuardPeerStat `json:"peers,omitempty"`
}

type NetworkProfileStat struct {
	Interface string   `json:"interface"`
	Backend   string   `json:"backend"`
	Supported bool     `json:"supported"`
	Managed   bool     `json:"managed"`
	Ownership string   `json:"ownership,omitempty"`
	Method    string   `json:"method,omitempty"`
	Address   string   `json:"address,omitempty"`
	Gateway   string   `json:"gateway,omitempty"`
	DNS       []string `json:"dns,omitempty"`
	Source    string   `json:"source,omitempty"`
	Error     string   `json:"error,omitempty"`
}

type Response struct {
	QuotaLimitBytes    uint64                `json:"quota_limit_bytes,omitempty"`
	QuotaUsedBytes     uint64                `json:"quota_used_bytes,omitempty"`
	UsageUsedBytes     int64                 `json:"usage_used_bytes,omitempty"`
	UsageReservedBytes int64                 `json:"usage_reserved_bytes,omitempty"`
	OK                 bool                  `json:"ok"`
	Message            string                `json:"message,omitempty"`
	Error              string                `json:"error,omitempty"`
	HelperVersion      string                `json:"helper_version,omitempty"`
	ProtocolVersion    int                   `json:"protocol_version,omitempty"`
	RollbackAvailable  bool                  `json:"rollback_available,omitempty"`
	RollbackVersion    string                `json:"rollback_version,omitempty"`
	FilesystemStats    []FilesystemStat      `json:"filesystem_stats,omitempty"`
	DiskHealth         []DiskHealthStat      `json:"disk_health,omitempty"`
	LVM                []LVMStat             `json:"lvm,omitempty"`
	WireGuardAvailable bool                  `json:"wireguard_available,omitempty"`
	WireGuardError     string                `json:"wireguard_error,omitempty"`
	WireGuardTunnels   []WireGuardTunnelStat `json:"wireguard_tunnels,omitempty"`
	NetworkBackend     string                `json:"network_backend,omitempty"`
	NetworkProfiles    []NetworkProfileStat  `json:"network_profiles,omitempty"`
	SMBAvailable       bool                  `json:"smb_available,omitempty"`
	SMBActive          bool                  `json:"smb_active,omitempty"`
	SMBError           string                `json:"smb_error,omitempty"`
	SMBConfiguredUsers []string              `json:"smb_configured_users,omitempty"`
	SMBHardQuotaReady  bool                  `json:"smb_hard_quota_ready"`
	SMBHardQuotaError  string                `json:"smb_hard_quota_error,omitempty"`
	DockerAvailable    bool                  `json:"docker_available"`
	DockerActive       bool                  `json:"docker_active"`
	DockerVersion      string                `json:"docker_version,omitempty"`
	DockerNetworkReady bool                  `json:"docker_network_ready"`
	DockerError        string                `json:"docker_error,omitempty"`
	ModuleState        string                `json:"module_state,omitempty"`
	ContainerID        string                `json:"container_id,omitempty"`
}

type Result struct {
	Status    string `json:"status"`
	Version   string `json:"version,omitempty"`
	Message   string `json:"message,omitempty"`
	Error     string `json:"error,omitempty"`
	UpdatedAt string `json:"updated_at"`
}
