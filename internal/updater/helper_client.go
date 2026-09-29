package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

const updaterSocketPath = "/run/home-ai-core-updater.sock"

var ErrHelperUpgradeRequired = errors.New("system updater helper is outdated; install the latest initial installer once")

type HelperInfo struct {
	Version           string `json:"version"`
	ProtocolVersion   int    `json:"protocol_version"`
	Available         bool   `json:"available"`
	Compatible        bool   `json:"compatible"`
	RollbackAvailable bool   `json:"rollback_available"`
	RollbackVersion   string `json:"rollback_version,omitempty"`
	Error             string `json:"error,omitempty"`
}

func queryUpdaterHelperInfo(ctx context.Context) (HelperInfo, error) {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", updaterSocketPath)
	if err != nil {
		return HelperInfo{Error: err.Error()}, err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(updaterhelper.Request{Operation: "info"}); err != nil {
		return HelperInfo{Error: err.Error()}, err
	}
	var response updaterhelper.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return HelperInfo{Error: err.Error()}, err
	}
	if !response.OK || response.ProtocolVersion == 0 {
		message := response.Error
		if message == "" {
			message = "installed helper does not support protocol discovery"
		}
		return HelperInfo{Available: true, Error: message}, ErrHelperUpgradeRequired
	}
	info := HelperInfo{
		Version:           response.HelperVersion,
		ProtocolVersion:   response.ProtocolVersion,
		Available:         true,
		Compatible:        response.ProtocolVersion >= updaterhelper.ProtocolVersion,
		RollbackAvailable: response.RollbackAvailable,
		RollbackVersion:   response.RollbackVersion,
	}
	if !info.Compatible {
		info.Error = fmt.Sprintf(
			"helper protocol %d is older than required protocol %d",
			response.ProtocolVersion,
			updaterhelper.ProtocolVersion,
		)
		return info, ErrHelperUpgradeRequired
	}
	return info, nil
}

func callUpdaterHelper(ctx context.Context, currentVersion, version string) error {
	if _, err := queryUpdaterHelperInfo(ctx); err != nil {
		return err
	}

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", updaterSocketPath)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(updaterhelper.Request{
		Operation:       "install",
		ProtocolVersion: updaterhelper.ProtocolVersion,
		Version:         version,
		CurrentVersion:  currentVersion,
	}); err != nil {
		return err
	}

	var response updaterhelper.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return err
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "updater helper rejected installation"
		}
		return errors.New(response.Error)
	}
	return nil
}

func callUpdaterRollback(ctx context.Context) (string, error) {
	info, err := queryUpdaterHelperInfo(ctx)
	if err != nil {
		return "", err
	}
	if !info.RollbackAvailable || info.RollbackVersion == "" {
		return "", errors.New("no rollback backup is available")
	}

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", updaterSocketPath)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(updaterhelper.Request{
		Operation:       "rollback",
		ProtocolVersion: updaterhelper.ProtocolVersion,
	}); err != nil {
		return "", err
	}
	var response updaterhelper.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return "", err
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "updater helper rejected rollback"
		}
		return "", errors.New(response.Error)
	}
	if response.RollbackVersion != "" {
		return response.RollbackVersion, nil
	}
	return info.RollbackVersion, nil
}
