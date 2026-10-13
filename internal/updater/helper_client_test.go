package updater

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestProtectUpdaterHelperConnTimesOutStalledRead(t *testing.T) {
	client, helper := net.Pipe()
	defer helper.Close()

	closeConn, err := protectUpdaterHelperConn(context.Background(), client, 30*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer closeConn()

	started := time.Now()
	_, err = client.Read(make([]byte, 1))
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("stalled helper read error = %v, want timeout", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("stalled helper took too long to time out")
	}
}

func TestProtectUpdaterHelperConnStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, helper := net.Pipe()
	defer helper.Close()

	closeConn, err := protectUpdaterHelperConn(ctx, client, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer closeConn()

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	started := time.Now()
	_, err = client.Read(make([]byte, 1))
	if err == nil {
		t.Fatal("canceled helper read unexpectedly succeeded")
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("context error = %v, want canceled", ctx.Err())
	}
	if time.Since(started) > time.Second {
		t.Fatal("helper read did not stop promptly on cancellation")
	}
}

func TestProtectUpdaterHelperConnHonorsContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	client, helper := net.Pipe()
	defer helper.Close()

	closeConn, err := protectUpdaterHelperConn(ctx, client, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer closeConn()

	started := time.Now()
	_, err = client.Read(make([]byte, 1))
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("expired deadline read error = %v, want timeout", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("context deadline was not enforced")
	}
}

func TestProtectUpdaterHelperConnRejectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client, helper := net.Pipe()
	defer helper.Close()

	cleanup, err := protectUpdaterHelperConn(ctx, client, time.Second)
	if cleanup != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cleanup = %v, error = %v, want canceled context", cleanup != nil, err)
	}
}
