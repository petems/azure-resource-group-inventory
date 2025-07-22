package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSpinnerStartStop(t *testing.T) {
	spinner := NewSpinner("testing spinner")

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	spinner.Start()
	time.Sleep(250 * time.Millisecond)
	spinner.Stop()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "testing spinner") {
		t.Errorf("expected spinner message in output, got %q", output)
	}

	frames := []string{"|", "/", "-", "\\"}
	foundFrame := false
	for _, f := range frames {
		if strings.Contains(output, f) {
			foundFrame = true
			break
		}
	}
	if !foundFrame {
		t.Errorf("expected spinner frames in output, got %q", output)
	}
}
