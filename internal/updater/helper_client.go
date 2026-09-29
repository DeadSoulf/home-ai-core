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
	Version         string `json:"version"`
	ProtocolVersion int    `json:"protocol_version"`
	Compatible      bool   `json:"compatible"`
	Error           string `json:"error,omitempty"`
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
		return HelperInfo{Error: message}, ErrHelperUpgradeRequired
	}
	info := HelperInfo{
		Version:         response.HelperVersion,
		ProtocolVersion: response.ProtocolVersion,
		Compatible:      response.ProtocolVersion >= updaterhelper.ProtocolVersion,
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

func callUpdaterHelper(ctx context.Context, version string) error {
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
