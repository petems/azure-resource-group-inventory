package main

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestSpinnerStartStop ensures the spinner outputs data and stops correctly
func TestSpinnerStartStop(t *testing.T) {
	spinner := NewSpinner("spinner test")

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	spinner.Start()
	time.Sleep(200 * time.Millisecond)
	spinner.Stop()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed to read spinner output: %v", err)
	}

	if spinner.active {
		t.Error("spinner should be inactive after Stop")
	}
	if !strings.Contains(buf.String(), "spinner test") {
		t.Error("expected spinner output to contain message")
	}
}

// TestFetchResourceGroupsSlowConnection simulates slower HTTP responses
func TestFetchResourceGroupsSlowConnection(t *testing.T) {
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			time.Sleep(50 * time.Millisecond)
			if strings.Contains(req.URL.Path, "resourcegroups") {
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
                        "value": [
                            {
                                "id": "/subscriptions/test/resourceGroups/slow-rg",
                                "name": "slow-rg",
                                "location": "westus",
                                "properties": {"provisioningState": "Succeeded"}
                            }
                        ]
                    }`)),
				}
				return resp, nil
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"value": []}`))}, nil
		},
	}

	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test",
			AccessToken:    "token",
			MaxConcurrency: 1,
			Porcelain:      true,
		},
		HTTPClient: mockClient,
	}

	start := time.Now()
	err := client.FetchResourceGroups()
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if duration < 50*time.Millisecond {
		t.Errorf("expected duration >= 50ms, got %v", duration)
	}
}

// FuzzValidateConcurrency ensures validateConcurrency never returns < 1
func FuzzValidateConcurrency(f *testing.F) {
	seeds := []int{-10, -1, 0, 1, 2, 5, 10}
	for _, v := range seeds {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, n int) {
		if out := validateConcurrency(n); out < 1 {
			t.Fatalf("validateConcurrency(%d) returned %d", n, out)
		}
	})
}

// FuzzCheckIfDefaultResourceGroup verifies CreatedBy is set for default groups
func FuzzCheckIfDefaultResourceGroup(f *testing.F) {
	seeds := []string{"DefaultResourceGroup-EUS", "my-rg"}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		info := checkIfDefaultResourceGroup(name)
		if info.IsDefault && info.CreatedBy == "" {
			t.Errorf("default resource group %s missing CreatedBy", name)
		}
	})
}
