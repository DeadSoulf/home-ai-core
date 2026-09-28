package updater

import (
	"context"
	"encoding/json"
	"errors"
	"net"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

const updaterSocketPath = "/run/home-ai-core-updater.sock"

func callUpdaterHelper(ctx context.Context, version string) error {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", updaterSocketPath)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(updaterhelper.Request{
		Operation: "install",
		Version:   version,
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
