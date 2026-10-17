package main

import "time"

// transferRate estimates throughput only from bytes observed by this process.
// It deliberately ignores elapsed time while the process is not copying.
// ETA is an approximation for the current file, not the whole sync run.
type transferRate struct {
	at    time.Time
	bytes int64
	speed int64
}

func (r *transferRate) observe(now time.Time, uploaded, total int64) (bytesPerSecond, etaSeconds int64) {
	if total < 0 {
		total = 0
	}
	if uploaded < 0 {
		uploaded = 0
	}
	if total > 0 && uploaded > total {
		uploaded = total
	}
	if r.at.IsZero() || uploaded < r.bytes || now.Before(r.at) {
		r.at, r.bytes, r.speed = now, uploaded, 0
		return 0, 0
	}
	elapsed := now.Sub(r.at)
	if elapsed >= 350*time.Millisecond && uploaded > r.bytes {
		instant := int64(float64(uploaded-r.bytes) / elapsed.Seconds())
		if r.speed == 0 {
			r.speed = instant
		} else {
			r.speed = (3*r.speed + instant) / 4
		}
		r.at, r.bytes = now, uploaded
	} else if elapsed >= 4*time.Second && uploaded == r.bytes {
		// Never display a stale nonzero throughput after a long stall.
		r.speed = 0
		r.at = now
	}
	if r.speed <= 0 || total <= uploaded {
		return 0, 0
	}
	remaining := total - uploaded
	eta := remaining / r.speed
	if remaining%r.speed != 0 {
		eta++
	}
	return r.speed, eta
}
