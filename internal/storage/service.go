package storage

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

const helperSocketPath = "/run/home-ai-core-updater.sock"

type Request struct {
	Operation  string
	Device     string
	Mountpoint string
	Filesystem string
	Label      string
	Confirm    string
}

func Execute(ctx context.Context, input Request) (string, error) {
	operation := strings.TrimSpace(input.Operation)
	switch operation {
	case "mount", "unmount", "format":
	default:
		return "", errors.New("unsupported storage operation")
	}

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", helperSocketPath)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	request := updaterhelper.Request{
		Operation:  "storage." + operation,
		Device:     strings.TrimSpace(input.Device),
		Mountpoint: strings.TrimSpace(input.Mountpoint),
		Filesystem: strings.TrimSpace(input.Filesystem),
		Label:      strings.TrimSpace(input.Label),
		Confirm:    strings.TrimSpace(input.Confirm),
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
	return response.Message, nil
}
