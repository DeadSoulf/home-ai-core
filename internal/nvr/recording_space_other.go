//go:build !linux

package nvr

import "errors"

func (osSpaceChecker) Space(string) (FilesystemSpace, error) {
	return FilesystemSpace{}, errors.New("filesystem space inspection is unavailable on this platform")
}
