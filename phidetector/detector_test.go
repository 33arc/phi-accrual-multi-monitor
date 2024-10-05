package phidetector

import (
	"testing"
	"time"
)

func TestPhiAccrualFailureDetector(t *testing.T) {
	failureDetector, err := NewBuilder().Build()
	if err != nil {
		t.Fatalf("Failed to create PhiAccrualFailureDetector: %v", err)
	}

	now := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

	for i := 0; i < 300; i++ {
		timestampMillis := now + int64(i*1000)

		if i > 290 {
			phi := failureDetector.Phi(timestampMillis)
			if i == 291 {
				if !(1 < phi && phi < 3) {
					t.Errorf("Expected 1 < phi < 3, got phi = %f", phi)
				}
				if !failureDetector.IsAvailable(timestampMillis) {
					t.Errorf("Expected detector to be available at i = %d", i)
				}
			} else if i == 292 {
				if !(3 < phi && phi < 8) {
					t.Errorf("Expected 3 < phi < 8, got phi = %f", phi)
				}
				if !failureDetector.IsAvailable(timestampMillis) {
					t.Errorf("Expected detector to be available at i = %d", i)
				}
			} else if i == 293 {
				if !(8 < phi && phi < 16) {
					t.Errorf("Expected 8 < phi < 16, got phi = %f", phi)
				}
				if !failureDetector.IsAvailable(timestampMillis) {
					t.Errorf("Expected detector to be available at i = %d", i)
				}
			} else if i == 294 {
				if !(16 < phi && phi < 30) {
					t.Errorf("Expected 16 < phi < 30, got phi = %f", phi)
				}
				if failureDetector.IsAvailable(timestampMillis) {
					t.Errorf("Expected detector to be unavailable at i = %d", i)
				}
			} else if i == 295 {
				if !(30 < phi && phi < 50) {
					t.Errorf("Expected 30 < phi < 50, got phi = %f", phi)
				}
				if failureDetector.IsAvailable(timestampMillis) {
					t.Errorf("Expected detector to be unavailable at i = %d", i)
				}
			} else if i == 296 {
				if !(50 < phi && phi < 70) {
					t.Errorf("Expected 50 < phi < 70, got phi = %f", phi)
				}
				if failureDetector.IsAvailable(timestampMillis) {
					t.Errorf("Expected detector to be unavailable at i = %d", i)
				}
			} else if i == 297 {
				if !(70 < phi && phi < 100) {
					t.Errorf("Expected 70 < phi < 100, got phi = %f", phi)
				}
				if failureDetector.IsAvailable(timestampMillis) {
					t.Errorf("Expected detector to be unavailable at i = %d", i)
				}
			} else {
				if !(100 < phi) {
					t.Errorf("Expected phi > 100, got phi = %f", phi)
				}
				if failureDetector.IsAvailable(timestampMillis) {
					t.Errorf("Expected detector to be unavailable at i = %d", i)
				}
			}
			continue
		} else if i > 200 {
			if i%5 == 0 {
				phi := failureDetector.Phi(timestampMillis)
				if !(0.1 < phi && phi < 0.5) {
					t.Errorf("Expected 0.1 < phi < 0.5, got phi = %f", phi)
				}
				if !failureDetector.IsAvailable(timestampMillis) {
					t.Errorf("Expected detector to be available at i = %d", i)
				}
				continue
			}
		}

		failureDetector.Heartbeat(timestampMillis)
		phi := failureDetector.Phi(timestampMillis)
		if !(phi < 0.1) {
			t.Errorf("Expected phi < 0.1, got phi = %f", phi)
		}
		if !failureDetector.IsAvailable(timestampMillis) {
			t.Errorf("Expected detector to be available at i = %d", i)
		}
	}
}
