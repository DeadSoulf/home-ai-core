package systeminfo

type Info struct {
	NodeID            string             `json:"node_id"`
	Hostname          string             `json:"hostname"`
	OS                string             `json:"os"`
	Kernel            string             `json:"kernel,omitempty"`
	Architecture      string             `json:"architecture"`
	CPU               CPUInfo            `json:"cpu"`
	Memory            MemoryInfo         `json:"memory"`
	UptimeSeconds     uint64             `json:"uptime_seconds,omitempty"`
	BlockDevices      []BlockDevice      `json:"block_devices"`
	BlockTree         []BlockNode        `json:"block_tree"`
	NetworkInterfaces []NetworkInterface `json:"network_interfaces"`
	GPUs              []GPU              `json:"gpus"`
}

type CPUInfo struct {
	Model        string  `json:"model,omitempty"`
	LogicalCPUs  int     `json:"logical_cpus"`
	UsagePercent float64 `json:"usage_percent"`
}

type MemoryInfo struct {
	TotalBytes     uint64 `json:"total_bytes,omitempty"`
	AvailableBytes uint64 `json:"available_bytes,omitempty"`
}

type BlockNode struct {
	Name        string      `json:"name"`
	DisplayName string      `json:"display_name,omitempty"`
	Path        string      `json:"path,omitempty"`
	Type        string      `json:"type"`
	Filesystem  string      `json:"filesystem,omitempty"`
	SizeBytes   uint64      `json:"size_bytes,omitempty"`
	FreeBytes        uint64      `json:"free_bytes,omitempty"`
	FreeKnown        bool        `json:"free_known"`
	UnallocatedBytes uint64      `json:"unallocated_bytes,omitempty"`
	PartitionTable   string      `json:"partition_table,omitempty"`
	Mountpoints      []string    `json:"mountpoints"`
	ParentName  string      `json:"parent_name,omitempty"`
	Label       string      `json:"label,omitempty"`
	UUID        string      `json:"uuid,omitempty"`
	Model       string      `json:"model,omitempty"`
	Vendor      string      `json:"vendor,omitempty"`
	Serial      string      `json:"serial,omitempty"`
	Transport   string      `json:"transport,omitempty"`
	Health      string      `json:"health,omitempty"`
	TemperatureC     *int    `json:"temperature_c,omitempty"`
	PowerOnHours     *uint64 `json:"power_on_hours,omitempty"`
	LifeRemainingPct *int    `json:"life_remaining_percent,omitempty"`
	SmartAvailable   bool    `json:"smart_available"`
	SmartError       string  `json:"smart_error,omitempty"`
	LVMVGName        string   `json:"lvm_vg_name,omitempty"`
	LVMLVName        string   `json:"lvm_lv_name,omitempty"`
	LVMActive        *bool    `json:"lvm_active,omitempty"`
	LVMVGSizeBytes   uint64   `json:"lvm_vg_size_bytes,omitempty"`
	LVMVGFreeBytes   uint64   `json:"lvm_vg_free_bytes,omitempty"`
	LVMDataPercent   *float64 `json:"lvm_data_percent,omitempty"`
	LVMMetadataPercent *float64 `json:"lvm_metadata_percent,omitempty"`
	Rotational  bool        `json:"rotational"`
	Removable   bool        `json:"removable"`
	System      bool        `json:"system"`
	Children    []BlockNode `json:"children"`
}

type BlockDevice struct {
	Name        string      `json:"name"`
	DisplayName string      `json:"display_name,omitempty"`
	Path       string      `json:"path"`
	MajorMinor string      `json:"major_minor,omitempty"`
	Vendor     string      `json:"vendor,omitempty"`
	Model      string      `json:"model,omitempty"`
	Serial     string      `json:"serial,omitempty"`
	SizeBytes  uint64      `json:"size_bytes,omitempty"`
	Rotational bool        `json:"rotational"`
	Removable  bool        `json:"removable"`
	System     bool        `json:"system"`
	Partitions []Partition `json:"partitions"`
}

type Partition struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	SizeBytes   uint64   `json:"size_bytes,omitempty"`
	Filesystem  string   `json:"filesystem,omitempty"`
	UUID        string   `json:"uuid,omitempty"`
	Label       string   `json:"label,omitempty"`
	Mountpoints []string `json:"mountpoints"`
}

type NetworkInterface struct {
	Name      string   `json:"name"`
	Index     int      `json:"index"`
	MAC       string   `json:"mac,omitempty"`
	MTU       int      `json:"mtu"`
	Up        bool     `json:"up"`
	Loopback  bool     `json:"loopback"`
	Multicast bool     `json:"multicast"`
	OperState string   `json:"oper_state,omitempty"`
	SpeedBPS  uint64   `json:"speed_bps,omitempty"`
	Addresses []string `json:"addresses"`
}

type GPU struct {
	Card       string `json:"card,omitempty"`
	Vendor     string `json:"vendor,omitempty"`
	Model      string `json:"model,omitempty"`
	VendorID   string `json:"vendor_id,omitempty"`
	DeviceID   string `json:"device_id,omitempty"`
	Class      string `json:"class,omitempty"`
	Driver     string `json:"driver,omitempty"`
	Modalias   string `json:"modalias,omitempty"`
	PCIAddress        string   `json:"pci_address,omitempty"`
	UtilizationPercent *float64 `json:"utilization_percent,omitempty"`
}
