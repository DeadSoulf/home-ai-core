package nvr

type MountChecker interface {
	Mounted(string) (bool, error)
}

type osMountChecker struct{}
