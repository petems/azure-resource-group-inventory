package main

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// countingHTTPClient tracks concurrent Do calls
type countingHTTPClient struct {
	maxConcurrent int32
	current       int32
}

func (c *countingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	v := atomic.AddInt32(&c.current, 1)
	for {
		max := atomic.LoadInt32(&c.maxConcurrent)
		if v > max {
			atomic.CompareAndSwapInt32(&c.maxConcurrent, max, v)
		} else {
			break
		}
	}
	time.Sleep(50 * time.Millisecond)
	atomic.AddInt32(&c.current, -1)
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"value": []}`)),
	}, nil
}

func TestRateLimitingWithSlowConnections(t *testing.T) {
	client := &AzureClient{
		Config: Config{
			SubscriptionID: "sub",
			AccessToken:    "token",
			MaxConcurrency: 2,
			Porcelain:      true,
		},
		HTTPClient: &countingHTTPClient{},
	}

	rgs := []ResourceGroup{
		{Name: "rg1"}, {Name: "rg2"}, {Name: "rg3"}, {Name: "rg4"}, {Name: "rg5"},
	}

	start := time.Now()
	client.processResourceGroupsConcurrentlyCSV(rgs)
	duration := time.Since(start)

	c := client.HTTPClient.(*countingHTTPClient)
	if c.maxConcurrent > int32(client.Config.MaxConcurrency) {
		t.Errorf("expected at most %d concurrent, got %d", client.Config.MaxConcurrency, c.maxConcurrent)
	}

	expectedMin := time.Duration(len(rgs)/client.Config.MaxConcurrency) * 50 * time.Millisecond
	if duration < expectedMin {
		t.Errorf("expected duration >= %v, got %v", expectedMin, duration)
	}
}

func TestSpinnerStartStop(t *testing.T) {
	s := NewSpinner("testing")
	go s.Start()
	time.Sleep(250 * time.Millisecond)
	s.Stop()
	if s.active {
		t.Error("spinner should not be active after Stop")
	}
}

func FuzzValidateConcurrency(f *testing.F) {
	seeds := []int{-10, 0, 1, 5, 100}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, n int) {
		result := validateConcurrency(n)
		if result < 1 {
			t.Fatalf("invalid result %d for input %d", result, n)
		}
	})
}

func FuzzCheckIfDefaultResourceGroup(f *testing.F) {
	seeds := []string{"DefaultResourceGroup-EUS", "custom-rg", "MC_rg_aks_eu", ""}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		info := checkIfDefaultResourceGroup(name)
		if info.IsDefault && info.CreatedBy == "" {
			t.Fatalf("CreatedBy empty for default group %s", name)
		}
	})
}
