package service

import (
	"testing"
	"time"
)

func TestResponsesPollSettings(t *testing.T) {
	t.Setenv("RESPONSES_POLL_WORKERS", "")
	t.Setenv("RESPONSES_POLL_INTERVAL_MS", "")
	workers, interval := responsesPollSettings()
	if workers != 32 || interval != 25*time.Millisecond {
		t.Fatalf("default poll settings = %d, %s", workers, interval)
	}

	t.Setenv("RESPONSES_POLL_WORKERS", "500")
	t.Setenv("RESPONSES_POLL_INTERVAL_MS", "1")
	workers, interval = responsesPollSettings()
	if workers != 64 || interval != 10*time.Millisecond {
		t.Fatalf("bounded poll settings = %d, %s", workers, interval)
	}
}
