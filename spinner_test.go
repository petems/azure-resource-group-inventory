package main

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"
)

// TestSpinnerStartStop verifies that the spinner prints frames and stops cleanly.
func TestSpinnerStartStop(t *testing.T) {
	sp := NewSpinner("testing spinner")

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	sp.Start()
	// Wait a short time to allow a few frames to print
	time.Sleep(250 * time.Millisecond)
	sp.Stop()

	// Restore stdout
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed reading output: %v", err)
	}
	output := buf.String()

	if sp.active {
		t.Error("spinner should not be active after Stop")
	}
	if output == "" {
		t.Error("expected spinner to produce output")
	}
}

// TestSpinnerStopWithoutStart ensures Stop does not block if Start was never called.
func TestSpinnerStopWithoutStart(t *testing.T) {
	sp := NewSpinner("no start")

	done := make(chan struct{})
	go func() {
		sp.Stop()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Stop blocked when spinner not started")
	}
}
