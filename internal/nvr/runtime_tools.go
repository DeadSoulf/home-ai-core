package nvr

import "os/exec"

func resolveRuntimeExecutable(cached, name string) string {
	if cached != "" {
		return cached
	}
	path, _ := exec.LookPath(name)
	return path
}
