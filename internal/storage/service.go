package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

const helperSocketPath = "/run/home-ai-core-updater.sock"

type Request struct {
	Operation      string
	Device         string
	Mountpoint     string
	Filesystem     string
	Label          string
	Confirm        string
	SizeMiB        uint64
	RootPath       string
	RelativePath   string
	ReservePercent int
}

func ensureCompatibleHelper(ctx context.Context) error {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", helperSocketPath)
	if err != nil {
		return fmt.Errorf("connect storage helper: %w", err)
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(updaterhelper.Request{Operation: "info"}); err != nil {
		return fmt.Errorf("query storage helper: %w", err)
	}
	var response updaterhelper.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return fmt.Errorf("query storage helper: %w", err)
	}
	if !response.OK || response.ProtocolVersion < updaterhelper.ProtocolVersion {
		return errors.New("system storage helper is outdated; install the latest initial installer once")
	}
	return nil
}

func Execute(ctx context.Context, input Request) (string, error) {
	operation := strings.TrimSpace(input.Operation)
	switch operation {
	case "mount", "unmount", "format", "partition.create", "partition.delete", "partition.delete_all", "label.rename",
		"nas.prepare_pool", "nas.prepare_folder", "nas.capacity_policy":
	default:
		return "", errors.New("unsupported storage operation")
	}
	if err := ensureCompatibleHelper(ctx); err != nil {
		return "", err
	}

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", helperSocketPath)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	request := updaterhelper.Request{
		Operation:       "storage." + operation,
		ProtocolVersion: updaterhelper.ProtocolVersion,
		Device:          strings.TrimSpace(input.Device),
		Mountpoint:      strings.TrimSpace(input.Mountpoint),
		Filesystem:      strings.TrimSpace(input.Filesystem),
		Label:           strings.TrimSpace(input.Label),
		Confirm:         strings.TrimSpace(input.Confirm),
		SizeMiB:         input.SizeMiB,
		RootPath:        strings.TrimSpace(input.RootPath),
		RelativePath:    strings.TrimSpace(input.RelativePath),
		ReservePercent:  input.ReservePercent,
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return "", err
	}

	var response updaterhelper.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return "", err
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "storage helper rejected operation"
		}
		return "", errors.New(response.Error)
	}
	InvalidateInspectionCache()
	return response.Message, nil
}

func PrepareNASPool(ctx context.Context, rootPath string) error {
	_, err := Execute(ctx, Request{
		Operation: "nas.prepare_pool",
		RootPath:  rootPath,
	})
	return err
}

func PrepareNASFolder(ctx context.Context, rootPath, relativePath string) error {
	_, err := Execute(ctx, Request{
		Operation:    "nas.prepare_folder",
		RootPath:     rootPath,
		RelativePath: relativePath,
	})
	return err
}

func ApplyNASCapacityPolicy(ctx context.Context, rootPath string, reservePercent int) error {
	_, err := Execute(ctx, Request{
		Operation:      "nas.capacity_policy",
		RootPath:       rootPath,
		ReservePercent: reservePercent,
	})
	return err
}
