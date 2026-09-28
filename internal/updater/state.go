package updater

import "time"

type Phase string

const (
	PhaseIdle        Phase = "idle"
	PhaseChecking    Phase = "checking"
	PhaseAvailable   Phase = "available"
	PhaseDownloading Phase = "downloading"
	PhaseReady       Phase = "ready"
	PhaseInstalling  Phase = "installing"
	PhaseRestarting  Phase = "restarting"
	PhaseSucceeded   Phase = "succeeded"
	PhaseFailed      Phase = "failed"
)

type State struct {
	Phase            Phase      `json:"phase"`
	CurrentVersion   string     `json:"current_version,omitempty"`
	AvailableVersion string     `json:"available_version,omitempty"`
	ProgressPercent  int        `json:"progress_percent,omitempty"`
	Message          string     `json:"message,omitempty"`
	Error            string     `json:"error,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at"`
	PublishedAt      *time.Time `json:"published_at,omitempty"`
	BundleSizeBytes  int64      `json:"bundle_size_bytes,omitempty"`
}

func NewState(currentVersion string) State {
	return State{
		Phase:          PhaseIdle,
		CurrentVersion: currentVersion,
		UpdatedAt:      time.Now().UTC(),
	}
}

func (s State) Terminal() bool {
	return s.Phase == PhaseSucceeded || s.Phase == PhaseFailed
}
