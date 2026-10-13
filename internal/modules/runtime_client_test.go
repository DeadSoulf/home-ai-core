package modules

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestModuleHelperIPCStalledReadTimesOut(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	cleanup, err := protectModuleHelperConn(context.Background(), client, 30*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	_, err = client.Read(make([]byte, 1))
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("stalled read error = %v, want network timeout", err)
	}
}

func TestModuleHelperIPCCancellationInterruptsRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, server := net.Pipe()
	defer server.Close()

	cleanup, err := protectModuleHelperConn(ctx, client, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	started := time.Now()
	_, err = client.Read(make([]byte, 1))
	if err == nil {
		t.Fatal("canceled module helper read unexpectedly succeeded")
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("context error = %v, want context canceled", ctx.Err())
	}
	if time.Since(started) > time.Second {
		t.Fatal("module helper read was not interrupted promptly")
	}
}

func TestModuleHelperIPCHonorsShorterContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
	defer cancel()

	client, server := net.Pipe()
	defer server.Close()

	cleanup, err := protectModuleHelperConn(ctx, client, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	_, err = client.Read(make([]byte, 1))
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("deadline read error = %v, want network timeout", err)
	}
}

func TestModuleHelperIPCRejectsAlreadyCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client, server := net.Pipe()
	defer server.Close()

	cleanup, err := protectModuleHelperConn(ctx, client, time.Second)
	if cleanup != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cleanup exists = %t, error = %v, want canceled context", cleanup != nil, err)
	}
}
