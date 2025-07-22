package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestMakeAzureRequestRateLimit verifies handling of HTTP 429 responses
func TestMakeAzureRequestRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limited"))
	}))
	defer server.Close()

	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-sub",
			AccessToken:    "token",
			Porcelain:      true, // disable spinner
		},
		HTTPClient: server.Client(),
	}

	_, err := client.makeAzureRequest(server.URL)
	if err == nil {
		t.Fatalf("expected error for 429 response, got nil")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("expected error to mention status 429, got %v", err)
	}
}

// TestSpinnerStartStop ensures spinner goroutine exits without leaking
func TestSpinnerStartStop(t *testing.T) {
	spinner := NewSpinner("testing")
	spinner.Start()
	time.Sleep(200 * time.Millisecond)
	spinner.Stop()
	if spinner.active {
		t.Error("spinner should not be active after Stop")
	}
	select {
	case <-spinner.done:
		// ok
	default:
		t.Error("spinner done channel should be closed")
	}
}

// TestFetchResourceGroupsSlowConnection simulates slow network responses
func TestFetchResourceGroupsSlowConnection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		if strings.Contains(r.URL.Path, "resourcegroups") && !strings.Contains(r.URL.Path, "resources") {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"value": [{"id": "/subscriptions/test/resourceGroups/slow-rg","name":"slow-rg","location":"eastus","properties":{"provisioningState":"Succeeded"}}]}`)
		} else {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"value": []}`)
		}
	}))
	defer server.Close()

	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-sub",
			AccessToken:    "token",
			MaxConcurrency: 1,
			Porcelain:      true,
		},
		HTTPClient: server.Client(),
	}

	start := time.Now()
	err := client.FetchResourceGroups()
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if elapsed < 200*time.Millisecond {
		t.Error("expected FetchResourceGroups to take at least the server delay")
	}
}

// FuzzCheckIfDefaultResourceGroup fuzzes the default group detection
func FuzzCheckIfDefaultResourceGroup(f *testing.F) {
	seeds := []string{"DefaultResourceGroup-EUS", "MC_rg_cluster_eastus", "", "foo", "123"}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		_ = checkIfDefaultResourceGroup(name)
	})
}
