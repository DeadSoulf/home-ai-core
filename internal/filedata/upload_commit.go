package filedata

import "os"

func installUploadPartByLink(partPath, target string) error {
	if err := os.Link(partPath, target); err != nil {
		return err
	}
	// The destination is committed once Link succeeds. A cleanup failure must
	// not turn that success into a retryable upload failure. Removing this extra
	// link cannot remove the committed destination.
	_ = os.Remove(partPath)
	return nil
}
