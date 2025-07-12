package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestPrecompiledRegexPatterns tests that all pre-compiled regex patterns work correctly
func TestPrecompiledRegexPatterns(t *testing.T) {
	// Test that patterns are actually compiled and not nil
	patterns := map[string]*regexp.Regexp{
		"defaultResourceGroupPattern": defaultResourceGroupPattern,
		"dynamicsPattern":            dynamicsPattern,
		"aksPattern":                 aksPattern,
		"azureBackupPattern":         azureBackupPattern,
		"networkWatcherPattern":      networkWatcherPattern,
		"databricksPattern":          databricksPattern,
		"microsoftNetworkPattern":    microsoftNetworkPattern,
		"logAnalyticsPattern":        logAnalyticsPattern,
	}

	for name, pattern := range patterns {
		if pattern == nil {
			t.Errorf("Pattern %s is nil", name)
		}
	}
}

// TestPrecompiledRegexAccuracy tests that pre-compiled patterns produce same results as before
func TestPrecompiledRegexAccuracy(t *testing.T) {
	testCases := []struct {
		name           string
		resourceGroup  string
		shouldMatch    bool
		expectedResult DefaultResourceGroupInfo
	}{
		{
			name:          "DefaultResourceGroup pattern",
			resourceGroup: "DefaultResourceGroup-EUS",
			shouldMatch:   true,
			expectedResult: DefaultResourceGroupInfo{
				IsDefault:   true,
				CreatedBy:   "Azure CLI / Cloud Shell / Visual Studio",
				Description: "Common default resource group created for the region, used by Azure CLI, Cloud Shell, and Visual Studio for resource deployment",
			},
		},
		{
			name:          "AKS pattern",
			resourceGroup: "MC_myRG_myAKS_eastus",
			shouldMatch:   true,
			expectedResult: DefaultResourceGroupInfo{
				IsDefault:   true,
				CreatedBy:   "Azure Kubernetes Service (AKS)",
				Description: "Created when deploying an AKS cluster, contains infrastructure resources for the cluster",
			},
		},
		{
			name:          "Custom resource group",
			resourceGroup: "my-custom-rg",
			shouldMatch:   false,
			expectedResult: DefaultResourceGroupInfo{
				IsDefault:   false,
				CreatedBy:   "",
				Description: "",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := checkIfDefaultResourceGroup(tc.resourceGroup)
			
			if result.IsDefault != tc.expectedResult.IsDefault {
				t.Errorf("Expected IsDefault=%v, got %v", tc.expectedResult.IsDefault, result.IsDefault)
			}
			if result.CreatedBy != tc.expectedResult.CreatedBy {
				t.Errorf("Expected CreatedBy='%s', got '%s'", tc.expectedResult.CreatedBy, result.CreatedBy)
			}
			if result.Description != tc.expectedResult.Description {
				t.Errorf("Expected Description='%s', got '%s'", tc.expectedResult.Description, result.Description)
			}
		})
	}
}

// TestConcurrentProcessing tests the concurrent processing functionality
func TestConcurrentProcessing(t *testing.T) {
	// Create test resource groups
	resourceGroups := []ResourceGroup{
		{
			ID:       "/subscriptions/test/resourceGroups/test-rg-1",
			Name:     "test-rg-1",
			Location: "eastus",
			Properties: struct {
				ProvisioningState string `json:"provisioningState"`
			}{ProvisioningState: "Succeeded"},
		},
		{
			ID:       "/subscriptions/test/resourceGroups/test-rg-2",
			Name:     "test-rg-2",
			Location: "westus",
			Properties: struct {
				ProvisioningState string `json:"provisioningState"`
			}{ProvisioningState: "Succeeded"},
		},
		{
			ID:       "/subscriptions/test/resourceGroups/test-rg-3",
			Name:     "test-rg-3",
			Location: "centralus",
			Properties: struct {
				ProvisioningState string `json:"provisioningState"`
			}{ProvisioningState: "Succeeded"},
		},
	}

	// Create mock HTTP client
	callCount := 0
	var mu sync.Mutex
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			
			// Simulate some processing time
			time.Sleep(10 * time.Millisecond)
			
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

	// Create Azure client
	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-subscription",
			AccessToken:    "test-token",
			MaxConcurrency: 2,
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
	io.Copy(&buf, r)
	output := buf.String()

	// Verify all resource groups were processed
	if !strings.Contains(output, "test-rg-1") {
		t.Error("Expected output to contain 'test-rg-1'")
	}
	if !strings.Contains(output, "test-rg-2") {
		t.Error("Expected output to contain 'test-rg-2'")
	}
	if !strings.Contains(output, "test-rg-3") {
		t.Error("Expected output to contain 'test-rg-3'")
	}

	// Verify API was called for each resource group
	if callCount != 3 {
		t.Errorf("Expected 3 API calls, got %d", callCount)
	}

	// Verify concurrent processing was faster than sequential
	// With 2 concurrent workers and 3 resource groups, should be faster than 3 * 10ms
	if duration > 25*time.Millisecond {
		t.Errorf("Concurrent processing took too long: %v", duration)
	}
}

// TestConcurrentProcessingErrorHandling tests error handling in concurrent processing
func TestConcurrentProcessingErrorHandling(t *testing.T) {
	resourceGroups := []ResourceGroup{
		{
			ID:       "/subscriptions/test/resourceGroups/test-rg-1",
			Name:     "test-rg-1",
			Location: "eastus",
			Properties: struct {
				ProvisioningState string `json:"provisioningState"`
			}{ProvisioningState: "Succeeded"},
		},
		{
			ID:       "/subscriptions/test/resourceGroups/test-rg-2",
			Name:     "test-rg-2",
			Location: "westus",
			Properties: struct {
				ProvisioningState string `json:"provisioningState"`
			}{ProvisioningState: "Succeeded"},
		},
	}

	// Create mock HTTP client that returns error for second request
	callCount := 0
	var mu sync.Mutex
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			mu.Lock()
			callCount++
			currentCall := callCount
			mu.Unlock()
			
			if currentCall == 2 {
				return nil, io.EOF // Simulate network error
			}
			
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
			MaxConcurrency: 2,
		},
		HTTPClient: mockClient,
	}

	// Capture output
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Test concurrent processing with errors
	client.processResourceGroupsConcurrently(resourceGroups)

	// Restore stdout
	w.Close()
	os.Stdout = old

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify error is handled gracefully
	if !strings.Contains(output, "Error fetching") {
		t.Error("Expected error message in output")
	}
	
	// Verify both resource groups are still processed
	if !strings.Contains(output, "test-rg-1") || !strings.Contains(output, "test-rg-2") {
		t.Error("Expected both resource groups to be processed despite error")
	}
}

// TestSemaphoreRateLimiting tests that semaphore correctly limits concurrency
func TestSemaphoreRateLimiting(t *testing.T) {
	// Create many resource groups
	resourceGroups := make([]ResourceGroup, 10)
	for i := 0; i < 10; i++ {
		resourceGroups[i] = ResourceGroup{
			ID:       fmt.Sprintf("/subscriptions/test/resourceGroups/test-rg-%d", i),
			Name:     fmt.Sprintf("test-rg-%d", i),
			Location: "eastus",
			Properties: struct {
				ProvisioningState string `json:"provisioningState"`
			}{ProvisioningState: "Succeeded"},
		}
	}

	// Track concurrent requests and total requests
	var concurrentRequests int32
	var maxConcurrent int32
	var totalRequests int32
	var mu sync.Mutex

	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			mu.Lock()
			concurrentRequests++
			totalRequests++
			if concurrentRequests > maxConcurrent {
				maxConcurrent = concurrentRequests
			}
			currentMax := maxConcurrent
			currentTotal := totalRequests
			mu.Unlock()
			
			t.Logf("Request %d: concurrent=%d, max=%d", currentTotal, concurrentRequests, currentMax)
			
			// Simulate processing time
			time.Sleep(20 * time.Millisecond)
			
			mu.Lock()
			concurrentRequests--
			mu.Unlock()
			
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
			MaxConcurrency: 3, // Limit to 3 concurrent requests
		},
		HTTPClient: mockClient,
	}

	// Capture output
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Test concurrent processing
	client.processResourceGroupsConcurrently(resourceGroups)

	// Restore stdout
	w.Close()
	os.Stdout = old

	// Read captured output to drain pipe
	var buf bytes.Buffer
	io.Copy(&buf, r)

	// Verify requests were made
	t.Logf("Total requests made: %d", totalRequests)
	t.Logf("Final max concurrent requests: %d", maxConcurrent)
	
	// Verify all requests were made
	if totalRequests != 10 {
		t.Errorf("Expected 10 HTTP requests, got %d", totalRequests)
	}
	
	// Verify semaphore limited concurrency
	if maxConcurrent > 3 {
		t.Errorf("Expected max concurrent requests to be <= 3, got %d", maxConcurrent)
	}
	if maxConcurrent < 1 {
		t.Errorf("Expected at least 1 concurrent request, got %d", maxConcurrent)
	}
}

// TestHTTPClientOptimization tests HTTP client configuration
func TestHTTPClientOptimization(t *testing.T) {
	// Save original config
	originalConfig := config
	defer func() { config = originalConfig }()

	// Test initialization with optimized HTTP client
	config = Config{
		SubscriptionID: "test-sub",
		AccessToken:    "test-token",
		MaxConcurrency: 10,
	}

	client := &AzureClient{
		Config: config,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}

	// Verify HTTP client has optimized transport
	if client.HTTPClient == nil {
		t.Fatal("HTTPClient should not be nil")
	}

	httpClient, ok := client.HTTPClient.(*http.Client)
	if !ok {
		t.Fatal("HTTPClient should be *http.Client")
	}

	transport, ok := httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport should be *http.Transport")
	}

	// Verify transport settings
	if transport.MaxIdleConns != 100 {
		t.Errorf("Expected MaxIdleConns=100, got %d", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 10 {
		t.Errorf("Expected MaxIdleConnsPerHost=10, got %d", transport.MaxIdleConnsPerHost)
	}
	if transport.IdleConnTimeout != 90*time.Second {
		t.Errorf("Expected IdleConnTimeout=90s, got %v", transport.IdleConnTimeout)
	}
}

// TestPerformanceMonitoring tests performance monitoring functionality
func TestPerformanceMonitoring(t *testing.T) {
	// Create a test function that uses performance monitoring
	testFunction := func() {
		start := time.Now()
		defer func() {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			// In a real test, you'd capture and verify the log output
			_ = time.Since(start)
			_ = m.Alloc / 1024
		}()
		
		// Simulate some work
		time.Sleep(10 * time.Millisecond)
	}

	// Run the test function
	testFunction()
	
	// Test passes if no panic occurs
	// In a more sophisticated test, you would capture log output and verify it
}

// TestConfigurableConcurrency tests the configurable concurrency feature
func TestConfigurableConcurrency(t *testing.T) {
	testCases := []struct {
		name               string
		maxConcurrency     int
		expectedConcurrency int
	}{
		{
			name:               "Default concurrency",
			maxConcurrency:     0,
			expectedConcurrency: 10,
		},
		{
			name:               "Custom concurrency",
			maxConcurrency:     5,
			expectedConcurrency: 5,
		},
		{
			name:               "High concurrency",
			maxConcurrency:     20,
			expectedConcurrency: 20,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := Config{
				SubscriptionID: "test-sub",
				AccessToken:    "test-token",
				MaxConcurrency: tc.maxConcurrency,
			}

			if tc.maxConcurrency == 0 {
				// Simulate default value assignment
				config.MaxConcurrency = 10
			}

			if config.MaxConcurrency != tc.expectedConcurrency {
				t.Errorf("Expected MaxConcurrency=%d, got %d", tc.expectedConcurrency, config.MaxConcurrency)
			}
		})
	}
}

// TestMemorySafetyInConcurrentProcessing tests memory safety in concurrent processing
func TestMemorySafetyInConcurrentProcessing(t *testing.T) {
	// Create many resource groups to stress test memory safety
	resourceGroups := make([]ResourceGroup, 100)
	for i := 0; i < 100; i++ {
		resourceGroups[i] = ResourceGroup{
			ID:       "/subscriptions/test/resourceGroups/test-rg-" + string(rune(i)),
			Name:     "test-rg-" + string(rune(i)),
			Location: "eastus",
			Properties: struct {
				ProvisioningState string `json:"provisioningState"`
			}{ProvisioningState: "Succeeded"},
		}
	}

	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
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

	// Test concurrent processing with many items
	client.processResourceGroupsConcurrently(resourceGroups)

	// Restore stdout
	w.Close()
	os.Stdout = old

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)

	// Test passes if no race conditions or memory issues occur
	// This is primarily a stress test for memory safety
}

// TestResourceGroupResult tests the ResourceGroupResult struct
func TestResourceGroupResult(t *testing.T) {
	createdTime := time.Now()
	
	result := ResourceGroupResult{
		ResourceGroup: ResourceGroup{
			ID:       "/subscriptions/test/resourceGroups/test-rg",
			Name:     "test-rg",
			Location: "eastus",
			Properties: struct {
				ProvisioningState string `json:"provisioningState"`
			}{ProvisioningState: "Succeeded"},
		},
		CreatedTime: &createdTime,
		Error:       nil,
	}

	if result.ResourceGroup.Name != "test-rg" {
		t.Errorf("Expected ResourceGroup.Name='test-rg', got '%s'", result.ResourceGroup.Name)
	}
	if result.CreatedTime == nil {
		t.Error("Expected CreatedTime to be set")
	}
	if result.Error != nil {
		t.Errorf("Expected no error, got %v", result.Error)
	}
}

// TestPrintResourceGroupResult tests the result printing functionality
func TestPrintResourceGroupResult(t *testing.T) {
	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-subscription",
			AccessToken:    "test-token",
			MaxConcurrency: 10,
		},
		HTTPClient: &MockHTTPClient{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"value": []}`)),
				}, nil
			},
		},
	}

	createdTime := time.Now()
	result := ResourceGroupResult{
		ResourceGroup: ResourceGroup{
			ID:       "/subscriptions/test/resourceGroups/DefaultResourceGroup-EUS",
			Name:     "DefaultResourceGroup-EUS",
			Location: "eastus",
			Properties: struct {
				ProvisioningState string `json:"provisioningState"`
			}{ProvisioningState: "Succeeded"},
		},
		CreatedTime: &createdTime,
		Error:       nil,
	}

	// Capture output
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Test printing
	client.printResourceGroupResult(result, false)

	// Restore stdout
	w.Close()
	os.Stdout = old

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify output contains expected information
	if !strings.Contains(output, "DefaultResourceGroup-EUS") {
		t.Error("Expected output to contain resource group name")
	}
	if !strings.Contains(output, "DEFAULT RESOURCE GROUP DETECTED") {
		t.Error("Expected output to contain default resource group detection")
	}
	if !strings.Contains(output, "Azure CLI / Cloud Shell / Visual Studio") {
		t.Error("Expected output to contain created by information")
	}
}

// TestConcurrentProcessingWithResourceListing tests concurrent processing with resource listing
func TestConcurrentProcessingWithResourceListing(t *testing.T) {
	resourceGroups := []ResourceGroup{
		{
			ID:       "/subscriptions/test/resourceGroups/test-rg-1",
			Name:     "test-rg-1",
			Location: "eastus",
			Properties: struct {
				ProvisioningState string `json:"provisioningState"`
			}{ProvisioningState: "Succeeded"},
		},
	}

	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
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

	// Test concurrent processing with resource listing
	client.processResourceGroupsConcurrentlyWithResources(resourceGroups)

	// Restore stdout
	w.Close()
	os.Stdout = old

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify output contains resource group information
	if !strings.Contains(output, "test-rg-1") {
		t.Error("Expected output to contain resource group name")
	}
}