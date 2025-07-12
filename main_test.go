package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Mock HTTP client for testing
type MockHTTPClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}

func TestMakeAzureRequest(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if the Authorization header is present
		if r.Header.Get("Authorization") == "" {
			t.Error("Authorization header is missing")
		}

		// Check if the Content-Type header is set correctly
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("Content-Type header is not set to application/json")
		}

		// Return a successful response
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"value": []}`)); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	}))
	defer server.Close()

	// Create Azure client with real HTTP client
	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-sub",
			AccessToken:    "test-token",
		},
		HTTPClient: &http.Client{},
	}

	// Make a request to the test server
	resp, err := client.makeAzureRequest(server.URL)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("Failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestMakeAzureRequestWithError(t *testing.T) {
	// Create a test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		if _, err := w.Write([]byte(`{"error": "Unauthorized"}`)); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	}))
	defer server.Close()

	// Create Azure client with real HTTP client
	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-sub",
			AccessToken:    "invalid-token",
		},
		HTTPClient: &http.Client{},
	}

	// Make a request to the test server
	_, err := client.makeAzureRequest(server.URL)
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}
}

func TestFetchResourceGroupCreatedTime(t *testing.T) {
	// Create mock HTTP client
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			// Check if the URL contains the resource group name
			if !strings.Contains(req.URL.Path, "test-rg") {
				t.Errorf("Expected resource group 'test-rg' in URL, got %s", req.URL.Path)
			}

			// Return mock response
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"value": [
						{
							"id": "/subscriptions/test/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test",
							"name": "test-storage",
							"type": "Microsoft.Storage/storageAccounts",
							"createdTime": "2023-01-01T12:00:00Z"
						},
						{
							"id": "/subscriptions/test/resourceGroups/test-rg/providers/Microsoft.Web/sites/test",
							"name": "test-app",
							"type": "Microsoft.Web/sites",
							"createdTime": "2023-01-02T12:00:00Z"
						}
					]
				}`)),
			}
			return resp, nil
		},
	}

	// Create Azure client with mock HTTP client
	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-subscription",
			AccessToken:    "test-token",
		},
		HTTPClient: mockClient,
	}

	// Test the function
	createdTime, err := client.fetchResourceGroupCreatedTime("test-rg")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if createdTime == nil {
		t.Fatal("Expected a created time, got nil")
	}

	// The earliest time should be 2023-01-01T12:00:00Z
	expectedTime, _ := time.Parse(time.RFC3339, "2023-01-01T12:00:00Z")
	if !createdTime.Equal(expectedTime) {
		t.Errorf("Expected created time to be %v, got %v", expectedTime, *createdTime)
	}
}

func TestFetchResourceGroupCreatedTimeWithNoResources(t *testing.T) {
	// Create mock HTTP client
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"value": []}`)),
			}
			return resp, nil
		},
	}

	// Create Azure client with mock HTTP client
	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-subscription",
			AccessToken:    "test-token",
		},
		HTTPClient: mockClient,
	}

	// Test the function
	createdTime, err := client.fetchResourceGroupCreatedTime("empty-rg")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if createdTime != nil {
		t.Errorf("Expected nil created time for empty resource group, got %v", *createdTime)
	}
}

func TestFetchResourceGroups(t *testing.T) {
	// Create mock HTTP client
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "resourcegroups") {
				// Return resource groups
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"value": [
							{
								"id": "/subscriptions/test/resourceGroups/test-rg-1",
								"name": "test-rg-1",
								"location": "eastus",
								"properties": {
									"provisioningState": "Succeeded"
								}
							},
							{
								"id": "/subscriptions/test/resourceGroups/test-rg-2",
								"name": "test-rg-2",
								"location": "westus",
								"properties": {
									"provisioningState": "Succeeded"
								}
							}
						]
					}`)),
				}
				return resp, nil
			} else {
				// Return resources for created time lookup
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"value": [
							{
								"id": "/subscriptions/test/resourceGroups/test-rg-1/providers/Microsoft.Storage/storageAccounts/test",
								"name": "test-storage",
								"type": "Microsoft.Storage/storageAccounts",
								"createdTime": "2023-01-01T12:00:00Z"
							}
						]
					}`)),
				}
				return resp, nil
			}
		},
	}

	// Create Azure client with mock HTTP client
	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-subscription",
			AccessToken:    "test-token",
		},
		HTTPClient: mockClient,
	}

	// Capture output
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Test the function
	err := client.FetchResourceGroups()

	// Restore stdout
	if err := w.Close(); err != nil {
		t.Errorf("Failed to close pipe writer: %v", err)
	}
	os.Stdout = old

	// Read captured output
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Errorf("Failed to copy output: %v", err)
	}

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Check if output contains expected resource groups
	outputStr := buf.String()
	if !strings.Contains(outputStr, "test-rg-1") {
		t.Error("Expected output to contain 'test-rg-1'")
	}
	if !strings.Contains(outputStr, "test-rg-2") {
		t.Error("Expected output to contain 'test-rg-2'")
	}
}

func TestConfigValidation(t *testing.T) {
	// Save original environment variables
	originalSubID := os.Getenv("AZURE_SUBSCRIPTION_ID")
	originalToken := os.Getenv("AZURE_ACCESS_TOKEN")

	defer func() {
		// Restore original environment variables
		if originalSubID != "" {
			if err := os.Setenv("AZURE_SUBSCRIPTION_ID", originalSubID); err != nil {
				t.Errorf("Failed to restore AZURE_SUBSCRIPTION_ID: %v", err)
			}
		} else {
			if err := os.Unsetenv("AZURE_SUBSCRIPTION_ID"); err != nil {
				t.Errorf("Failed to unset AZURE_SUBSCRIPTION_ID: %v", err)
			}
		}
		if originalToken != "" {
			if err := os.Setenv("AZURE_ACCESS_TOKEN", originalToken); err != nil {
				t.Errorf("Failed to restore AZURE_ACCESS_TOKEN: %v", err)
			}
		} else {
			if err := os.Unsetenv("AZURE_ACCESS_TOKEN"); err != nil {
				t.Errorf("Failed to unset AZURE_ACCESS_TOKEN: %v", err)
			}
		}
	}()

	// Test with missing subscription ID
	if err := os.Unsetenv("AZURE_SUBSCRIPTION_ID"); err != nil {
		t.Errorf("Failed to unset AZURE_SUBSCRIPTION_ID: %v", err)
	}
	if err := os.Unsetenv("AZURE_ACCESS_TOKEN"); err != nil {
		t.Errorf("Failed to unset AZURE_ACCESS_TOKEN: %v", err)
	}

	// Reset config
	config.SubscriptionID = ""
	config.AccessToken = ""

	// Test that configuration validation catches missing values
	if config.SubscriptionID != "" {
		t.Error("Expected empty subscription ID")
	}
	if config.AccessToken != "" {
		t.Error("Expected empty access token")
	}
}

func TestTimeHandling(t *testing.T) {
	// Test time parsing and comparison
	time1, err := time.Parse(time.RFC3339, "2023-01-01T12:00:00Z")
	if err != nil {
		t.Fatalf("Failed to parse time: %v", err)
	}

	time2, err := time.Parse(time.RFC3339, "2023-01-02T12:00:00Z")
	if err != nil {
		t.Fatalf("Failed to parse time: %v", err)
	}

	// Test that time1 is before time2
	if !time1.Before(time2) {
		t.Error("Expected time1 to be before time2")
	}

	// Test finding the earliest time
	var earliestTime *time.Time
	times := []*time.Time{&time2, &time1}

	for _, t := range times {
		if t != nil {
			if earliestTime == nil || t.Before(*earliestTime) {
				earliestTime = t
			}
		}
	}

	if earliestTime == nil {
		t.Error("Expected to find an earliest time")
		return
	}

	if !earliestTime.Equal(time1) {
		t.Error("Expected earliest time to be time1")
	}
}

func TestInvalidJSON(t *testing.T) {
	// Create mock HTTP client that returns invalid JSON
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`invalid json`)),
			}
			return resp, nil
		},
	}

	// Create Azure client with mock HTTP client
	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-subscription",
			AccessToken:    "test-token",
		},
		HTTPClient: mockClient,
	}

	// Test FetchResourceGroups with invalid JSON
	err := client.FetchResourceGroups()
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}

	// Test fetchResourceGroupCreatedTime with invalid JSON
	_, err = client.fetchResourceGroupCreatedTime("test-rg")
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}
