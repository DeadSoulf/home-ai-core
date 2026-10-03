//go:build !linux

package nvr

func (osMountChecker) Mounted(string) (bool, error) {
	return false, nil
}
