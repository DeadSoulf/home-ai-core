package windowsclient

import "fmt"

// DestinationConflictError reports a remote path that exists but does not
// match the local transfer snapshot. Callers may choose an explicit policy;
// CopyTransfer itself never overwrites the destination.
type DestinationConflictError struct {
	Path   string
	Reason string
}

func (e *DestinationConflictError) Error() string {
	if e == nil {
		return "destination conflict"
	}
	if e.Reason == "" {
		return fmt.Sprintf("destination %s conflicts with the selected source", e.Path)
	}
	return fmt.Sprintf("destination %s %s", e.Path, e.Reason)
}

func newDestinationConflict(path, reason string) error {
	return &DestinationConflictError{Path: path, Reason: reason}
}
