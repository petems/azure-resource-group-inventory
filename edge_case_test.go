package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"testing/quick"
	"time"
)

// TestMakeAzureRequestTimeout verifies that makeAzureRequest handles slow connections
func TestMakeAzureRequestTimeout_Edge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"value":[]}`))
	}))
	defer server.Close()

	client := &AzureClient{
		Config:     Config{AccessToken: "tok"},
		HTTPClient: &http.Client{Timeout: 10 * time.Millisecond},
	}

	_, err := client.makeAzureRequest(server.URL)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// TestSpinnerStartStop ensures spinner can start and stop around a slow operation
func TestSpinnerStartStop_Edge(t *testing.T) {
	s := NewSpinner("testing")

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	s.Start()
	time.Sleep(250 * time.Millisecond)
	s.Stop()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	if buf.Len() == 0 {
		t.Error("expected spinner output")
	}

	if s.active {
		t.Error("spinner should not be active after Stop")
	}
}

// TestValidateConcurrencyQuickCheck performs fuzz-like validation of concurrency handling
func TestValidateConcurrencyQuickCheck(t *testing.T) {
	fn := func(n int) bool {
		v := validateConcurrency(n)
		if n < 1 {
			return v == 1
		}
		return v == n
	}
	if err := quick.Check(fn, nil); err != nil {
		t.Errorf("quick check failed: %v", err)
	}
}

// TestCheckIfDefaultResourceGroupFuzz ensures stability of default detection for random strings
func TestCheckIfDefaultResourceGroupFuzz(t *testing.T) {
	fn := func(name string) bool {
		r1 := checkIfDefaultResourceGroup(name)
		r2 := checkIfDefaultResourceGroup(name)
		return reflect.DeepEqual(r1, r2)
	}
	if err := quick.Check(fn, nil); err != nil {
		t.Errorf("quick check failed: %v", err)
	}
}
