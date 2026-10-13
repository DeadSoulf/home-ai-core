package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/DeadSoulf/home-ai-core/internal/updater"
	"golang.org/x/sys/unix"
)

const trustedKeyPath = "/etc/home-ai-core/update-trusted.pub"
const archiveLimit int64 = 512 << 20

// Walk with openat(O_NOFOLLOW), pinning every directory descriptor. Core may
// mutate its input directory, but cannot redirect root through a symlink.
func openWithoutSymlinks(path string, directory bool) (*os.File, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("absolute canonical path required")
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, part := range parts {
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
		if i < len(parts)-1 || directory {
			flags |= unix.O_DIRECTORY
		}
		next, err := unix.Openat(fd, part, flags, 0)
		_ = unix.Close(fd)
		if err != nil {
			return nil, err
		}
		fd = next
	}
	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !directory && !info.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("update input is not a regular file")
	}
	return f, nil
}

// Every ancestor of trusted material must be root-owned and not writable by
// Core or any other unprivileged account. This makes path validation stable.
func requireRootOwnedPath(path string) error {
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != 0 || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("untrusted root path %s", p)
		}
		if p == "/" {
			break
		}
	}
	return nil
}

func ensureTrustedUpdateRoot() error {
	if err := requireRootOwnedPath(filepath.Dir(trustedUpdateRoot)); err != nil {
		return err
	}
	if err := os.Mkdir(trustedUpdateRoot, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return requireRootOwnedPath(trustedUpdateRoot)
}

func stageSignedUpdate(ctx context.Context, version string) (string, func(), error) {
	if !validVersion(version) {
		return "", nil, errors.New("invalid update version")
	}
	if err := ensureTrustedUpdateRoot(); err != nil {
		return "", nil, err
	}
	if err := requireRootOwnedPath(trustedKeyPath); err != nil {
		return "", nil, fmt.Errorf("install a root-owned Ed25519 trust key at %s before updating: %w", trustedKeyPath, err)
	}
	key, err := readUpdateInput(trustedKeyPath, 4096)
	if err != nil {
		return "", nil, err
	}
	return stageSignedUpdateAt(ctx, updateRoot, trustedUpdateRoot, key, version, runtime.GOARCH)
}

// The boundary wrapper supplies a protected root and a pinned key. The pure
// staging operation is separately testable without root privileges.
func stageSignedUpdateAt(ctx context.Context, incoming, privateRoot string, key []byte, version, arch string) (string, func(), error) {
	name := fmt.Sprintf("home-ai-core-update_%s_%s.tar.gz", version, arch)
	if filepath.Base(name) != name {
		return "", nil, errors.New("unsafe archive name")
	}
	checksum, err := readUpdateInput(filepath.Join(incoming, name+".sha256"), 4096)
	if err != nil {
		return "", nil, err
	}
	signature, err := readUpdateInput(filepath.Join(incoming, name+".sha256.sig"), 4096)
	if err != nil {
		return "", nil, fmt.Errorf("signed update required (including development releases): %w", err)
	}
	expected, err := updater.VerifySignedChecksum(checksum, signature, key, name)
	if err != nil {
		return "", nil, err
	}
	dir, err := os.MkdirTemp(privateRoot, "verified-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	success := false
	defer func() {
		if !success {
			cleanup()
		}
	}()
	input, err := openWithoutSymlinks(filepath.Join(incoming, name), false)
	if err != nil {
		return "", nil, err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return "", nil, err
	}
	if info.Size() <= 0 || info.Size() > archiveLimit {
		return "", nil, errors.New("update archive exceeds size bounds")
	}
	archive := filepath.Join(dir, "archive.tar.gz")
	output, err := os.OpenFile(archive, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", nil, err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(contextReader{ctx, input}, archiveLimit+1))
	syncErr := output.Sync()
	closeErr := output.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		return "", nil, err
	}
	if n > archiveLimit || hex.EncodeToString(hash.Sum(nil)) != expected {
		return "", nil, errors.New("signed update archive digest mismatch")
	}
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	prepared := filepath.Join(dir, "payload")
	if err := os.Mkdir(prepared, 0o700); err != nil {
		return "", nil, err
	}
	if err := updater.ExtractVerifiedArchive(archive, prepared, version, arch); err != nil {
		return "", nil, err
	}
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	success = true
	return prepared, cleanup, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(b)
}

func readUpdateInput(path string, limit int64) ([]byte, error) {
	f, err := openWithoutSymlinks(path, false)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("update metadata exceeds limit")
	}
	return data, nil
}

// Results are untrusted status messages, not install authorization. Use a
// fresh descriptor-relative temporary file; never chown/follow a supplied path.
func writeUntrustedResult(dir string, uid, gid int, data []byte) error {
	root, err := openWithoutSymlinks(dir, true)
	if err != nil {
		return err
	}
	defer root.Close()
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	name := ".install-result-" + hex.EncodeToString(random[:])
	fd, err := unix.Openat(int(root.Fd()), name, unix.O_CREAT|unix.O_EXCL|unix.O_WRONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	defer unix.Unlinkat(int(root.Fd()), name, 0)
	f := os.NewFile(uintptr(fd), name)
	if err := f.Chown(uid, gid); err != nil {
		f.Close()
		return err
	}
	_, we := f.Write(data)
	se := f.Sync()
	ce := f.Close()
	if err := errors.Join(we, se, ce); err != nil {
		return err
	}
	if err := unix.Renameat(int(root.Fd()), name, int(root.Fd()), "install-result.json"); err != nil {
		return err
	}
	return unix.Fsync(int(root.Fd()))
}
