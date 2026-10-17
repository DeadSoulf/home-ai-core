package main

import (
	"testing"
	"time"
)

func TestTransferRate(t *testing.T) {
	t0 := time.Unix(1, 0)
	var rate transferRate
	if speed, eta := rate.observe(t0, 0, 5_000); speed != 0 || eta != 0 {
		t.Fatalf("initial estimate = %d bytes/s, %ds", speed, eta)
	}
	if speed, eta := rate.observe(t0.Add(time.Second), 1000, 5000); speed != 1000 || eta != 4 {
		t.Fatalf("estimate = %d bytes/s, %ds, want 1000 bytes/s, 4s", speed, eta)
	}
	if speed, eta := rate.observe(t0.Add(2*time.Second), 2000, 5000); speed != 1000 || eta != 3 {
		t.Fatalf("second estimate = %d bytes/s, %ds", speed, eta)
	}
	if speed, eta := rate.observe(t0.Add(6*time.Second), 2000, 5000); speed != 0 || eta != 0 {
		t.Fatalf("stalled estimate = %d bytes/s, %ds", speed, eta)
	}
	if speed, eta := rate.observe(t0.Add(7*time.Second), 100, 1000); speed != 0 || eta != 0 {
		t.Fatalf("reset estimate = %d bytes/s, %ds", speed, eta)
	}
}

func TestTransferRateResumedAndZeroSize(t *testing.T) {
	var rate transferRate
	t0 := time.Now()
	rate.observe(t0, 4_000, 10_000)
	speed, eta := rate.observe(t0.Add(2*time.Second), 6_000, 10_000)
	if speed != 1_000 || eta != 4 {
		t.Fatalf("resumed speed/eta = %d/%d", speed, eta)
	}
	speed, eta = rate.observe(t0.Add(3*time.Second), 0, 0)
	if speed != 0 || eta != 0 {
		t.Fatalf("empty file speed/eta = %d/%d", speed, eta)
	}
}
