package helperipc

import (
	"context"
	"errors"
	"net"
	"time"
)

// Dial opens a privileged helper connection and bounds the entire request/
// response exchange. DialContext by itself only limits connection setup.
func Dial(ctx context.Context, socketPath string, timeout time.Duration) (net.Conn, func(), error) {
	if timeout <= 0 {
		return nil, nil, errors.New("helper IPC timeout must be positive")
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, nil, err
	}
	cleanup, err := protect(ctx, conn, timeout)
	if err != nil {
		return nil, nil, err
	}
	return conn, cleanup, nil
}

func protect(ctx context.Context, conn net.Conn, timeout time.Duration) (func(), error) {
	if err := ctx.Err(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return nil, err
	}
	stopCancellation := context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Now())
	})
	return func() {
		stopCancellation()
		_ = conn.Close()
	}, nil
}
