package main

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// Called only by the root Debian post-install script. Descriptor-relative
// creation refuses symlinks left in a directory writable by the old Core.
func preparePrivateState(uid, gid int) error {
	if err := requireRootOwnedPath("/var/lib"); err != nil {
		return err
	}
	parent, err := openWithoutSymlinks("/var/lib", true)
	if err != nil {
		return err
	}
	defer parent.Close()
	state, err := prepareDirectoryAt(parent, "home-ai-core", uid, gid, 0o700)
	if err != nil {
		return err
	}
	defer state.Close()
	update, err := prepareDirectoryAt(state, "update", uid, gid, 0o700)
	if err != nil {
		return err
	}
	update.Close()
	modules, err := prepareDirectoryAt(parent, "home-ai-modules", 0, gid, 0o750)
	if err != nil {
		return err
	}
	modules.Close()
	return ensureTrustedUpdateRoot()
}

func prepareDirectoryAt(parent *os.File, name string, uid, gid int, mode uint32) (*os.File, error) {
	if name == "" || name == "." || name == ".." {
		return nil, errors.New("invalid state directory")
	}
	if err := unix.Mkdirat(int(parent.Fd()), name, mode); err != nil && !errors.Is(err, unix.EEXIST) {
		return nil, err
	}
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), name)
	if err := f.Chown(uid, gid); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Chmod(os.FileMode(mode)); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
