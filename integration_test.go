package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestIntegrationOptimizedFetchResourceGroups tests the full optimized flow
func TestIntegrationOptimizedFetchResourceGroups(t *testing.T) {
	// Create mock HTTP client
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "resourcegroups") {
				// Return resource groups including default ones
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"value": [
							{
								"id": "/subscriptions/test/resourceGroups/DefaultResourceGroup-EUS",
								"name": "DefaultResourceGroup-EUS",
								"location": "eastus",
								"properties": {
									"provisioningState": "Succeeded"
								}
							},
							{
								"id": "/subscriptions/test/resourceGroups/MC_myRG_myAKS_eastus",
								"name": "MC_myRG_myAKS_eastus",
								"location": "eastus",
								"properties": {
									"provisioningState": "Succeeded"
								}
							},
							{
								"id": "/subscriptions/test/resourceGroups/my-custom-rg",
								"name": "my-custom-rg",
								"location": "westus",
								"properties": {
									"provisioningState": "Succeeded"
								}
							}
						]
					}`)),
				}, nil
			} else {
				// Return resources for created time lookup
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"value": [
							{
								"id": "/subscriptions/test/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test",
								"name": "test-storage",
								"type": "Microsoft.Storage/storageAccounts",
								"createdTime": "2023-01-01T12:00:00Z"
							}
						]
					}`)),
				}, nil
			}
		},
	}

	// Create Azure client with optimized settings
	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-subscription",
			AccessToken:    "test-token",
			MaxConcurrency: 5,
		},
		HTTPClient: mockClient,
	}

	// Capture output and logs
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Also capture logs
	oldLogFlags := log.Flags()
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer func() {
		log.SetFlags(oldLogFlags)
		log.SetOutput(os.Stderr)
	}()

	// Test the full optimized flow
	start := time.Now()
	err := client.FetchResourceGroups()
	duration := time.Since(start)

	// Restore stdout
	w.Close()
	os.Stdout = old

	// Read captured output
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()
	logOutput := logBuf.String()

	// Verify no errors occurred
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify all resource groups were processed
	if !strings.Contains(output, "DefaultResourceGroup-EUS") {
		t.Error("Expected output to contain 'DefaultResourceGroup-EUS'")
	}
	if !strings.Contains(output, "MC_myRG_myAKS_eastus") {
		t.Error("Expected output to contain 'MC_myRG_myAKS_eastus'")
	}
	if !strings.Contains(output, "my-custom-rg") {
		t.Error("Expected output to contain 'my-custom-rg'")
	}

	// Verify default resource group detection worked
	if !strings.Contains(output, "DEFAULT RESOURCE GROUP DETECTED") {
		t.Error("Expected output to contain 'DEFAULT RESOURCE GROUP DETECTED'")
	}
	if !strings.Contains(output, "Azure CLI / Cloud Shell / Visual Studio") {
		t.Error("Expected output to contain 'Azure CLI / Cloud Shell / Visual Studio'")
	}
	if !strings.Contains(output, "Azure Kubernetes Service (AKS)") {
		t.Error("Expected output to contain 'Azure Kubernetes Service (AKS)'")
	}

	// Verify performance monitoring worked
	if !strings.Contains(logOutput, "Operation completed in") {
		t.Error("Expected log to contain performance monitoring output")
	}
	if !strings.Contains(logOutput, "Memory usage:") {
		t.Error("Expected log to contain memory usage information")
	}

	// Verify execution was reasonably fast (should be much faster with concurrency)
	if duration > 1*time.Second {
		t.Errorf("Expected fast execution with concurrency, took %v", duration)
	}
}

// TestRaceConditionDetection tests for race conditions in concurrent processing
func TestRaceConditionDetection(t *testing.T) {
	// This test runs with the race detector enabled to catch race conditions
	// Run with: go test -race

	// Create many resource groups to increase chance of race conditions
	resourceGroups := make([]ResourceGroup, 50)
	for i := 0; i < 50; i++ {
		resourceGroups[i] = ResourceGroup{
			ID:       fmt.Sprintf("/subscriptions/test/resourceGroups/test-rg-%d", i),
			Name:     fmt.Sprintf("test-rg-%d", i),
			Location: "eastus",
			Properties: struct {
				ProvisioningState string `json:"provisioningState"`
			}{ProvisioningState: "Succeeded"},
		}
	}

	// Create mock HTTP client with variable response times
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			// Variable sleep to increase chance of race conditions
			time.Sleep(time.Duration(1+len(req.URL.Path)%5) * time.Millisecond)

			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"value": [
						{
							"id": "/subscriptions/test/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test",
							"name": "test-storage",
							"type": "Microsoft.Storage/storageAccounts",
							"createdTime": "2023-01-01T12:00:00Z"
						}
					]
				}`)),
			}, nil
		},
	}

	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-subscription",
			AccessToken:    "test-token",
			MaxConcurrency: 10,
		},
		HTTPClient: mockClient,
	}

	// Capture output
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Test concurrent processing multiple times to catch race conditions
	for i := 0; i < 5; i++ {
		client.processResourceGroupsConcurrently(resourceGroups)
	}

	// Restore stdout
	w.Close()
	os.Stdout = old

	// Read captured output
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	// Test passes if no race conditions are detected
	// The race detector will fail the test if races are found
}

// TestPerformanceMonitoringIntegration tests performance monitoring integration
func TestPerformanceMonitoringIntegration(t *testing.T) {
	// Create mock HTTP client
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "resourcegroups") {
				return &http.Response{
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
							}
						]
					}`)),
				}, nil
			} else {
				// Simulate some processing time
				time.Sleep(10 * time.Millisecond)
				return &http.Response{
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
				}, nil
			}
		},
	}

	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-subscription",
			AccessToken:    "test-token",
			MaxConcurrency: 5,
		},
		HTTPClient: mockClient,
	}

	// Capture logs
	var logBuf bytes.Buffer
	originalOutput := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(originalOutput)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Test FetchResourceGroups with performance monitoring
	err := client.FetchResourceGroups()

	// Restore stdout
	w.Close()
	os.Stdout = old

	// Read outputs
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	logOutput := logBuf.String()

	// Verify no errors
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify performance monitoring output
	if !strings.Contains(logOutput, "Operation completed in") {
		t.Error("Expected performance monitoring to log execution time")
	}
	if !strings.Contains(logOutput, "Memory usage:") {
		t.Error("Expected performance monitoring to log memory usage")
	}

	// Verify the log contains reasonable values
	if !strings.Contains(logOutput, "ms") && !strings.Contains(logOutput, "µs") && !strings.Contains(logOutput, "s") {
		t.Error("Expected timing information in performance log")
	}
}

// TestOptimizedRegexPerformance tests that optimized regex performs better
func TestOptimizedRegexPerformance(t *testing.T) {
	testNames := []string{
		"DefaultResourceGroup-EUS",
		"MC_myRG_myAKS_eastus",
		"AzureBackupRG_eastus_1",
		"NetworkWatcherRG",
		"databricks-rg-workspace-123",
		"microsoft-network",
		"LogAnalyticsDefaultResources",
		"DynamicsDeployments",
		"my-custom-resource-group",
		"prod-web-app-rg",
	}

	// Test optimized version (current implementation)
	start := time.Now()
	for i := 0; i < 1000; i++ {
		for _, name := range testNames {
			checkIfDefaultResourceGroup(name)
		}
	}
	optimizedDuration := time.Since(start)

	// Test should complete reasonably quickly
	if optimizedDuration > 100*time.Millisecond {
		t.Errorf("Optimized regex took too long: %v", optimizedDuration)
	}
}

// TestConcurrentProcessingScalability tests scalability of concurrent processing
func TestConcurrentProcessingScalability(t *testing.T) {
	scales := []int{1, 5, 10, 25, 50}

	for _, scale := range scales {
		t.Run(fmt.Sprintf("Scale_%d", scale), func(t *testing.T) {
			// Create resource groups
			resourceGroups := make([]ResourceGroup, scale)
			for i := 0; i < scale; i++ {
				resourceGroups[i] = ResourceGroup{
					ID:       fmt.Sprintf("/subscriptions/test/resourceGroups/test-rg-%d", i),
					Name:     fmt.Sprintf("test-rg-%d", i),
					Location: "eastus",
					Properties: struct {
						ProvisioningState string `json:"provisioningState"`
					}{ProvisioningState: "Succeeded"},
				}
			}

			// Create mock HTTP client
			var requestCount int
			var mu sync.Mutex
			mockClient := &MockHTTPClient{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					mu.Lock()
					requestCount++
					mu.Unlock()

					// Simulate API response time
					time.Sleep(5 * time.Millisecond)

					return &http.Response{
						StatusCode: http.StatusOK,
						Body: io.NopCloser(strings.NewReader(`{
							"value": [
								{
									"id": "/subscriptions/test/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test",
									"name": "test-storage",
									"type": "Microsoft.Storage/storageAccounts",
									"createdTime": "2023-01-01T12:00:00Z"
								}
							]
						}`)),
					}, nil
				},
			}

			client := &AzureClient{
				Config: Config{
					SubscriptionID: "test-subscription",
					AccessToken:    "test-token",
					MaxConcurrency: 10,
				},
				HTTPClient: mockClient,
			}

			// Capture output
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Test concurrent processing
			start := time.Now()
			client.processResourceGroupsConcurrently(resourceGroups)
			duration := time.Since(start)

			// Restore stdout
			w.Close()
			os.Stdout = old

			// Read captured output
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)

			// Verify all requests were made
			if requestCount != scale {
				t.Errorf("Expected %d requests, got %d", scale, requestCount)
			}

			// Verify performance scales appropriately
			// With 10 concurrent workers, larger scales should not be much slower
			expectedMaxDuration := time.Duration(scale/10+1) * 10 * time.Millisecond
			if duration > expectedMaxDuration {
				t.Errorf("Processing took too long for scale %d: %v (expected < %v)", scale, duration, expectedMaxDuration)
			}
		})
	}
}

// TestErrorHandlingInOptimizedFlow tests error handling in the optimized flow
func TestErrorHandlingInOptimizedFlow(t *testing.T) {
	// Create mock HTTP client that returns errors
	callCount := 0
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			if strings.Contains(req.URL.Path, "resourcegroups") {
				return &http.Response{
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
				}, nil
			} else {
				// Return error for resource fetching
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(strings.NewReader(`{"error": "Internal Server Error"}`)),
				}, nil
			}
		},
	}

	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-subscription",
			AccessToken:    "test-token",
			MaxConcurrency: 5,
		},
		HTTPClient: mockClient,
	}

	// Capture output
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Test FetchResourceGroups with errors
	err := client.FetchResourceGroups()

	// Restore stdout
	w.Close()
	os.Stdout = old

	// Read captured output
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Verify no fatal errors (should handle gracefully)
	if err != nil {
		t.Fatalf("Expected no fatal error, got %v", err)
	}

	// Verify resource groups were still processed
	if !strings.Contains(output, "test-rg-1") {
		t.Error("Expected output to contain 'test-rg-1'")
	}
	if !strings.Contains(output, "test-rg-2") {
		t.Error("Expected output to contain 'test-rg-2'")
	}

	// Verify error was handled gracefully
	if !strings.Contains(output, "Error fetching") {
		t.Error("Expected error message in output")
	}
}

// TestConfigurationIntegration tests integration of configuration options
func TestConfigurationIntegration(t *testing.T) {
	// Test different configuration scenarios
	testCases := []struct {
		name           string
		maxConcurrency int
		expectError    bool
	}{
		{
			name:           "Low concurrency",
			maxConcurrency: 1,
			expectError:    false,
		},
		{
			name:           "Default concurrency",
			maxConcurrency: 10,
			expectError:    false,
		},
		{
			name:           "High concurrency",
			maxConcurrency: 50,
			expectError:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &MockHTTPClient{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"value": []}`)),
					}, nil
				},
			}

			client := &AzureClient{
				Config: Config{
					SubscriptionID: "test-subscription",
					AccessToken:    "test-token",
					MaxConcurrency: tc.maxConcurrency,
				},
				HTTPClient: mockClient,
			}

			// Verify configuration
			if client.Config.MaxConcurrency != tc.maxConcurrency {
				t.Errorf("Expected MaxConcurrency=%d, got %d", tc.maxConcurrency, client.Config.MaxConcurrency)
			}

			// Test that it works with the configuration
			resourceGroups := []ResourceGroup{
				{
					ID:       "/subscriptions/test/resourceGroups/test-rg",
					Name:     "test-rg",
					Location: "eastus",
					Properties: struct {
						ProvisioningState string `json:"provisioningState"`
					}{ProvisioningState: "Succeeded"},
				},
			}

			// Capture output
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Test processing with configuration
			client.processResourceGroupsConcurrently(resourceGroups)

			// Restore stdout
			w.Close()
			os.Stdout = old

			// Read captured output
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)

			// Test passes if no panics or errors occur
		})
	}
}
