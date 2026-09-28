package coreupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"
)

type helperRequest struct {
	Operation     string `json:"operation"`
	PackagePath   string `json:"package_path"`
	SHA256        string `json:"sha256"`
	Version       string `json:"version"`
	DebianVersion string `json:"debian_version"`
	Architecture  string `json:"architecture"`
}

type helperResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

func callHelper(ctx context.Context, socketPath string, request helperRequest) error {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return fmt.Errorf("connect update helper: %w", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(10 * time.Minute))
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return fmt.Errorf("send update helper request: %w", err)
	}

	var response helperResponse
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		// During a successful self-update the package postinst restarts Core.
		// The current process normally terminates before this path is observed.
		if errors.Is(err, net.ErrClosed) {
			return err
		}
		return fmt.Errorf("read update helper response: %w", err)
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "update helper rejected request"
		}
		return errors.New(response.Error)
	}
	return nil
}
