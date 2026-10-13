package helperipc

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestProtectTimesOutOnStalledRead(t *testing.T) {
	client, helper := net.Pipe()
	defer helper.Close()
	cleanup, err := protect(context.Background(), client, 30*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	_, err = client.Read(make([]byte, 1))
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("stalled read error = %v, want timeout", err)
	}
}

func TestProtectCancelsBlockedRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, helper := net.Pipe()
	defer helper.Close()
	cleanup, err := protect(ctx, client, time.Hour)
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
	if err == nil || time.Since(started) > time.Second {
		t.Fatalf("cancellation did not interrupt helper read: %v", err)
	}
}

func TestProtectHonorsShorterContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	client, helper := net.Pipe()
	defer helper.Close()
	cleanup, err := protect(ctx, client, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	_, err = client.Read(make([]byte, 1))
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("deadline error = %v, want timeout", err)
	}
}

func TestProtectRejectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client, helper := net.Pipe()
	defer helper.Close()

	cleanup, err := protect(ctx, client, time.Second)
	if cleanup != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cleanup exists = %t, error = %v", cleanup != nil, err)
	}
}

func TestDialRejectsNonPositiveTimeout(t *testing.T) {
	if _, _, err := Dial(context.Background(), "/unused", 0); err == nil {
		t.Fatal("zero timeout was accepted")
	}
}
