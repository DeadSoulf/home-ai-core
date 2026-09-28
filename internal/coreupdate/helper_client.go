package coreupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/updatehelper"
)

func callHelper(ctx context.Context, socketPath string, request updatehelper.Request) error {
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

	var response updatehelper.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
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
