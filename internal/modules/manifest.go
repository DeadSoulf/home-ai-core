package modules

type Manifest struct {
	SchemaVersion int              `json:"schema_version"`
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Description   string           `json:"description,omitempty"`
	Version       string           `json:"version"`
	Core          string           `json:"core"`
	Dependencies  []Dependency     `json:"dependencies,omitempty"`
	Conflicts     []string         `json:"conflicts,omitempty"`
	Permissions   []string         `json:"permissions,omitempty"`
	Capabilities  Capabilities     `json:"capabilities,omitempty"`
	Host          HostRequirements `json:"host,omitempty"`
	Runtime       RuntimeContract   `json:"runtime,omitempty"`
	API           APIContribution  `json:"api,omitempty"`
	Events        EventContract    `json:"events,omitempty"`
	UI            UIContract       `json:"ui,omitempty"`
	Lifecycle     []string         `json:"lifecycle"`
}

type Dependency struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type Capabilities struct {
	Requires []string `json:"requires,omitempty"`
	Provides []string `json:"provides,omitempty"`
}

type HostRequirements struct {
	Architectures []string `json:"architectures,omitempty"`
	Packages      []string `json:"packages,omitempty"`
}

type RuntimeContract struct {
	Driver       string `json:"driver,omitempty"`
	Image        string `json:"image,omitempty"`
	InternalPort int    `json:"internal_port,omitempty"`
	HealthPath   string `json:"health_path,omitempty"`
}

type APIContribution struct {
	Namespace string `json:"namespace,omitempty"`
}

type EventContract struct {
	Publishes  []string `json:"publishes,omitempty"`
	Subscribes []string `json:"subscribes,omitempty"`
}

type UIContract struct {
	Navigation []NavigationItem `json:"navigation,omitempty"`
}

type NavigationItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Route string `json:"route"`
	Icon  string `json:"icon,omitempty"`
	Order int    `json:"order,omitempty"`
}

type Registered struct {
	Manifest Manifest `json:"manifest"`
	Status   string   `json:"status"`
	Error    string   `json:"error,omitempty"`
}
