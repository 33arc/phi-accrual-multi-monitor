package phidetector

import (
	"testing"
	"time"
)

func TestDetector(t *testing.T) {
	d := New(10, 5)
	now := time.Now()

	// Simulate regular pings
	for i := 0; i < 20; i++ {
		d.Ping(now)
		now = now.Add(time.Second)
	}

	phi := d.Phi(now)
	if phi <= 0 {
		t.Errorf("Expected positive phi value, got %f", phi)
	}

	// Simulate a missed heartbeat
	now = now.Add(10 * time.Second)
	phi = d.Phi(now)
	if phi <= 1 {
		t.Errorf("Expected higher phi value after missed heartbeat, got %f", phi)
	}
}
