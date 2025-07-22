package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestSpinnerStartStop verifies spinner activation and termination.
func TestSpinnerStartStop(t *testing.T) {
	s := NewSpinner("testing")
	if s.active {
		t.Fatal("spinner should not be active initially")
	}
	s.Start()
	time.Sleep(50 * time.Millisecond)
	if !s.active {
		t.Error("spinner should be active after Start")
	}
	s.Stop()
	if s.active {
		t.Error("spinner should not be active after Stop")
	}
	select {
	case <-s.done:
		// ok
	case <-time.After(100 * time.Millisecond):
		t.Error("spinner Stop did not signal done")
	}
}

// TestProcessResourceGroupsRateLimiting ensures concurrency never exceeds MaxConcurrency.
func TestProcessResourceGroupsRateLimiting(t *testing.T) {
	const maxConc = 2
	rgs := []ResourceGroup{
		{Name: "rg1"}, {Name: "rg2"}, {Name: "rg3"}, {Name: "rg4"},
	}

	var mu sync.Mutex
	active := 0
	maxObserved := 0
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			mu.Lock()
			active++
			if active > maxObserved {
				maxObserved = active
			}
			mu.Unlock()
			time.Sleep(50 * time.Millisecond)
			mu.Lock()
			active--
			mu.Unlock()
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"value":[]}`))}, nil
		},
	}

	ac := &AzureClient{
		Config:     Config{SubscriptionID: "sub", AccessToken: "tok", MaxConcurrency: maxConc, Porcelain: true},
		HTTPClient: mockClient,
	}

	ac.processResourceGroupsConcurrently(rgs)

	if maxObserved > maxConc {
		t.Errorf("expected max %d concurrent calls, got %d", maxConc, maxObserved)
	}
}

// TestMakeAzureRequestTimeout simulates a slow connection that exceeds the client timeout.
func TestMakeAzureRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ac := &AzureClient{
		Config:     Config{SubscriptionID: "sub", AccessToken: "tok", Porcelain: true},
		HTTPClient: &http.Client{Timeout: 10 * time.Millisecond},
	}

	_, err := ac.makeAzureRequest(server.URL)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// FuzzValidateConcurrency ensures validateConcurrency never returns less than 1.
func FuzzValidateConcurrency(f *testing.F) {
	seeds := []int{0, -5, 1, 10}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, n int) {
		res := validateConcurrency(n)
		if res < 1 {
			t.Fatalf("invalid result %d for input %d", res, n)
		}
	})
}
