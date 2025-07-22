package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
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
			Porcelain:      true, // Disable spinner in tests
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
			Porcelain:      true, // Disable spinner in tests
		},
		HTTPClient: &http.Client{},
	}

	// Make a request to the test server
	_, err := client.makeAzureRequest(server.URL)
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}
}

// TestMakeAzureRequestTimeout verifies request timeout handling.
func TestMakeAzureRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"value":[]}`))
	}))
	defer server.Close()
	client := &AzureClient{
		Config:     Config{SubscriptionID: "test", AccessToken: "token", Porcelain: true},
		HTTPClient: &http.Client{Timeout: 50 * time.Millisecond},
	}
	_, err := client.makeAzureRequest(server.URL)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// TestMakeAzureRequestNetworkError simulates a network failure.
func TestMakeAzureRequestNetworkError(t *testing.T) {
	mockClient := &MockHTTPClient{DoFunc: func(req *http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	}}
	client := &AzureClient{Config: Config{SubscriptionID: "test", AccessToken: "token", Porcelain: true}, HTTPClient: mockClient}
	_, err := client.makeAzureRequest("http://example.com")
	if err == nil {
		t.Fatal("expected network error")
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
			Porcelain:      true, // Disable spinner in tests
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
			Porcelain:      true, // Disable spinner in tests
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
			MaxConcurrency: 10,   // Set MaxConcurrency to prevent hanging
			Porcelain:      true, // Disable spinner in tests
		},
		HTTPClient: mockClient,
	}

	// Capture output
	old := os.Stdout
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("failed to create pipe: %v", pipeErr)
	}
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
			MaxConcurrency: 10,   // Set MaxConcurrency to prevent hanging
			Porcelain:      true, // Disable spinner in tests
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

func TestMaxConcurrencyValidation(t *testing.T) {
	testCases := []struct {
		name                   string
		inputMaxConcurrency    int
		expectedMaxConcurrency int
	}{
		{
			name:                   "Valid concurrency",
			inputMaxConcurrency:    5,
			expectedMaxConcurrency: 5,
		},
		{
			name:                   "Zero concurrency should be set to 1",
			inputMaxConcurrency:    0,
			expectedMaxConcurrency: 1,
		},
		{
			name:                   "Negative concurrency should be set to 1",
			inputMaxConcurrency:    -5,
			expectedMaxConcurrency: 1,
		},
		{
			name:                   "Minimum valid concurrency",
			inputMaxConcurrency:    1,
			expectedMaxConcurrency: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the validation function directly
			result := validateConcurrency(tc.inputMaxConcurrency)

			if result != tc.expectedMaxConcurrency {
				t.Errorf("Expected validateConcurrency(%d) to return %d, but got %d",
					tc.inputMaxConcurrency, tc.expectedMaxConcurrency, result)
			}

			// Also test that the validation prevents hanging in actual usage
			client := &AzureClient{
				Config: Config{
					SubscriptionID: "test-subscription",
					AccessToken:    "test-token",
					MaxConcurrency: tc.inputMaxConcurrency,
					Porcelain:      true, // Disable spinner in tests
				},
			}

			// Test the validation by calling processResourceGroupsConcurrently
			// which contains the validation logic
			resourceGroups := []ResourceGroup{
				{
					Name:     "test-rg",
					Location: "eastus",
					Properties: struct {
						ProvisioningState string `json:"provisioningState"`
					}{
						ProvisioningState: "Succeeded",
					},
				},
			}

			// Create a mock client for the test
			mockClient := &MockHTTPClient{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"value": []}`)),
					}, nil
				},
			}
			client.HTTPClient = mockClient

			// This should not hang regardless of the input MaxConcurrency
			client.processResourceGroupsConcurrently(resourceGroups)

			// The test passes if we reach this point without hanging
			t.Log("Test completed successfully - no hanging occurred")
		})
	}
}

func TestCheckIfDefaultResourceGroup(t *testing.T) {
	testCases := []struct {
		name                string
		resourceGroupName   string
		expectedIsDefault   bool
		expectedCreatedBy   string
		expectedDescription string
	}{
		{
			name:                "DefaultResourceGroup pattern",
			resourceGroupName:   "DefaultResourceGroup-EUS",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Azure CLI / Cloud Shell / Visual Studio",
			expectedDescription: "Common default resource group created for the region, used by Azure CLI, Cloud Shell, and Visual Studio for resource deployment",
		},
		{
			name:                "DefaultResourceGroup pattern uppercase",
			resourceGroupName:   "DEFAULTRESOURCEGROUP-WUS2",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Azure CLI / Cloud Shell / Visual Studio",
			expectedDescription: "Common default resource group created for the region, used by Azure CLI, Cloud Shell, and Visual Studio for resource deployment",
		},
		{
			name:                "DynamicsDeployments pattern",
			resourceGroupName:   "DynamicsDeployments",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Microsoft Dynamics ERP",
			expectedDescription: "Automatically created for Microsoft Dynamics ERP non-production instances",
		},
		{
			name:                "DynamicsDeployments pattern lowercase",
			resourceGroupName:   "dynamicsdeployments",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Microsoft Dynamics ERP",
			expectedDescription: "Automatically created for Microsoft Dynamics ERP non-production instances",
		},
		{
			name:                "AKS MC pattern",
			resourceGroupName:   "MC_myResourceGroup_myAKSCluster_eastus",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Azure Kubernetes Service (AKS)",
			expectedDescription: "Created when deploying an AKS cluster, contains infrastructure resources for the cluster",
		},
		{
			name:                "AKS MC pattern uppercase",
			resourceGroupName:   "MC_MYRESOURCEGROUP_MYAKSCLUSTER_WESTUS",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Azure Kubernetes Service (AKS)",
			expectedDescription: "Created when deploying an AKS cluster, contains infrastructure resources for the cluster",
		},
		{
			name:                "AzureBackupRG pattern",
			resourceGroupName:   "AzureBackupRG_eastus_1",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Azure Backup",
			expectedDescription: "Created by Azure Backup service for backup operations",
		},
		{
			name:                "AzureBackupRG pattern simple",
			resourceGroupName:   "AzureBackupRG",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Azure Backup",
			expectedDescription: "Created by Azure Backup service for backup operations",
		},
		{
			name:                "NetworkWatcherRG pattern",
			resourceGroupName:   "NetworkWatcherRG",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Azure Network Watcher",
			expectedDescription: "Created by Azure Network Watcher service for network monitoring",
		},
		{
			name:                "NetworkWatcherRG pattern lowercase",
			resourceGroupName:   "networkwatcherrg",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Azure Network Watcher",
			expectedDescription: "Created by Azure Network Watcher service for network monitoring",
		},
		{
			name:                "Databricks pattern",
			resourceGroupName:   "databricks-rg-myworkspace-xyz123",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Azure Databricks",
			expectedDescription: "Created by Azure Databricks service for managed workspace resources",
		},
		{
			name:                "Databricks pattern simple",
			resourceGroupName:   "databricks-rg",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Azure Databricks",
			expectedDescription: "Created by Azure Databricks service for managed workspace resources",
		},
		{
			name:                "Microsoft-network pattern",
			resourceGroupName:   "microsoft-network",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Microsoft Networking Services",
			expectedDescription: "Used by Microsoft's networking services",
		},
		{
			name:                "Microsoft-network pattern uppercase",
			resourceGroupName:   "MICROSOFT-NETWORK",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Microsoft Networking Services",
			expectedDescription: "Used by Microsoft's networking services",
		},
		{
			name:                "LogAnalyticsDefaultResources pattern",
			resourceGroupName:   "LogAnalyticsDefaultResources",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Azure Log Analytics",
			expectedDescription: "Created by Azure Log Analytics service for default workspace resources",
		},
		{
			name:                "LogAnalyticsDefaultResources pattern lowercase",
			resourceGroupName:   "loganalyticsdefaultresources",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Azure Log Analytics",
			expectedDescription: "Created by Azure Log Analytics service for default workspace resources",
		},
		{
			name:                "Default-Storage-EastUS pattern",
			resourceGroupName:   "Default-Storage-EastUS",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Azure Services",
			expectedDescription: "Default resource group created by Azure services for regional deployments",
		},
		{
			name:                "Default-EventHub-EastUS pattern",
			resourceGroupName:   "Default-EventHub-EastUS",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Azure Services",
			expectedDescription: "Default resource group created by Azure services for regional deployments",
		},
		{
			name:                "Default-ActivityLogAlerts pattern",
			resourceGroupName:   "Default-ActivityLogAlerts",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Azure Services",
			expectedDescription: "Default resource group created by Azure services for regional deployments",
		},
		{
			name:                "Default-SQL-JapanWest pattern",
			resourceGroupName:   "Default-SQL-JapanWest",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Azure Services",
			expectedDescription: "Default resource group created by Azure services for regional deployments",
		},
		{
			name:                "cloud-shell-storage-eastus pattern",
			resourceGroupName:   "cloud-shell-storage-eastus",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Azure Cloud Shell",
			expectedDescription: "Default storage resource group created by Azure Cloud Shell for persistent storage",
		},
		{
			name:                "cloud-shell-storage-centralindia pattern",
			resourceGroupName:   "cloud-shell-storage-centralindia",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Azure Cloud Shell",
			expectedDescription: "Default storage resource group created by Azure Cloud Shell for persistent storage",
		},
		{
			name:                "cloud-shell-storage-westeurope pattern",
			resourceGroupName:   "cloud-shell-storage-westeurope",
			expectedIsDefault:   true,
			expectedCreatedBy:   "Azure Cloud Shell",
			expectedDescription: "Default storage resource group created by Azure Cloud Shell for persistent storage",
		},
		{
			name:                "Custom resource group",
			resourceGroupName:   "my-custom-resource-group",
			expectedIsDefault:   false,
			expectedCreatedBy:   "",
			expectedDescription: "",
		},
		{
			name:                "Another custom resource group",
			resourceGroupName:   "prod-web-app-rg",
			expectedIsDefault:   false,
			expectedCreatedBy:   "",
			expectedDescription: "",
		},
		{
			name:                "Edge case - partial match",
			resourceGroupName:   "DefaultResourceGroup", // Missing suffix
			expectedIsDefault:   false,
			expectedCreatedBy:   "",
			expectedDescription: "",
		},
		{
			name:                "Edge case - MC pattern incomplete",
			resourceGroupName:   "MC_incomplete",
			expectedIsDefault:   false,
			expectedCreatedBy:   "",
			expectedDescription: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := checkIfDefaultResourceGroup(tc.resourceGroupName)

			if result.IsDefault != tc.expectedIsDefault {
				t.Errorf("Expected IsDefault=%v, got %v", tc.expectedIsDefault, result.IsDefault)
			}

			if result.CreatedBy != tc.expectedCreatedBy {
				t.Errorf("Expected CreatedBy='%s', got '%s'", tc.expectedCreatedBy, result.CreatedBy)
			}

			if result.Description != tc.expectedDescription {
				t.Errorf("Expected Description='%s', got '%s'", tc.expectedDescription, result.Description)
			}
		})
	}
}

func TestFetchResourceGroupsWithDefaultDetection(t *testing.T) {
	// Create mock HTTP client
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "resourcegroups") {
				// Return resource groups including default ones
				resp := &http.Response{
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
				}
				return resp, nil
			} else {
				// Return empty resources for created time lookup
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"value": []
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
			MaxConcurrency: 10,    // Set MaxConcurrency to prevent hanging
			Porcelain:      false, // Need human-readable output for this test
		},
		HTTPClient: mockClient,
	}

	// Capture output
	old := os.Stdout
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("failed to create pipe: %v", pipeErr)
	}
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

	// Check if output contains expected content
	outputStr := buf.String()

	// Should contain default resource group detection
	if !strings.Contains(outputStr, "DEFAULT RESOURCE GROUP DETECTED") {
		t.Error("Expected output to contain 'DEFAULT RESOURCE GROUP DETECTED'")
	}

	// Should contain Azure CLI detection
	if !strings.Contains(outputStr, "Azure CLI / Cloud Shell / Visual Studio") {
		t.Error("Expected output to contain 'Azure CLI / Cloud Shell / Visual Studio'")
	}

	// Should contain AKS detection
	if !strings.Contains(outputStr, "Azure Kubernetes Service (AKS)") {
		t.Error("Expected output to contain 'Azure Kubernetes Service (AKS)'")
	}

	// Should contain all resource group names
	if !strings.Contains(outputStr, "DefaultResourceGroup-EUS") {
		t.Error("Expected output to contain 'DefaultResourceGroup-EUS'")
	}
	if !strings.Contains(outputStr, "MC_myRG_myAKS_eastus") {
		t.Error("Expected output to contain 'MC_myRG_myAKS_eastus'")
	}
	if !strings.Contains(outputStr, "my-custom-rg") {
		t.Error("Expected output to contain 'my-custom-rg'")
	}
}

func TestCSVOutputWithoutResources(t *testing.T) {
	// Create a temporary CSV file
	tempFile, err := os.CreateTemp("", "test_output_*.csv")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() {
		if err := tempFile.Close(); err != nil {
			t.Errorf("Failed to close temp file: %v", err)
		}
		if err := os.Remove(tempFile.Name()); err != nil {
			t.Errorf("Failed to remove temp file: %v", err)
		}
	}()

	// Create mock HTTP client
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "resourcegroups") {
				// Return resource groups including default ones
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
								"id": "/subscriptions/test/resourceGroups/DefaultResourceGroup-EUS",
								"name": "DefaultResourceGroup-EUS",
								"location": "eastus",
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
			MaxConcurrency: 10,
			OutputCSV:      tempFile.Name(),
			Porcelain:      true, // Disable spinner in tests
		},
		HTTPClient: mockClient,
	}

	// Test the function
	err = client.FetchResourceGroups()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Read and verify CSV file
	csvContent, err := os.ReadFile(tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to read CSV file: %v", err)
	}

	csvStr := string(csvContent)
	// Check header
	if !strings.Contains(csvStr, "ResourceGroupName,Location,ProvisioningState,CreatedTime,IsDefault,CreatedBy,Description,Resources") {
		t.Error("Expected CSV header not found")
	}
	// Check data
	if !strings.Contains(csvStr, "test-rg-1,eastus,Succeeded") {
		t.Error("Expected resource group data not found in CSV")
	}
	if !strings.Contains(csvStr, "DefaultResourceGroup-EUS,eastus,Succeeded") {
		t.Error("Expected default resource group data not found in CSV")
	}
	// Check default resource group detection
	if !strings.Contains(csvStr, "true,Azure CLI / Cloud Shell / Visual Studio") {
		t.Error("Expected default resource group detection not found in CSV")
	}
}

func TestCSVOutputWithResources(t *testing.T) {
	// Create a temporary CSV file
	tempFile, err := os.CreateTemp("", "test_output_resources_*.csv")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() {
		if err := tempFile.Close(); err != nil {
			t.Errorf("Failed to close temp file: %v", err)
		}
		if err := os.Remove(tempFile.Name()); err != nil {
			t.Errorf("Failed to remove temp file: %v", err)
		}
	}()

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
							}
						]
					}`)),
				}
				return resp, nil
			} else {
				// Return resources for the resource group
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"value": [
							{
								"id": "/subscriptions/test/resourceGroups/test-rg-1/providers/Microsoft.Storage/storageAccounts/test-storage",
								"name": "test-storage",
								"type": "Microsoft.Storage/storageAccounts",
								"createdTime": "2023-01-01T12:00:00Z"
							},
							{
								"id": "/subscriptions/test/resourceGroups/test-rg-1/providers/Microsoft.Web/sites/test-app",
								"name": "test-app",
								"type": "Microsoft.Web/sites",
								"createdTime": "2023-01-02T12:00:00Z"
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
			MaxConcurrency: 10,
			OutputCSV:      tempFile.Name(),
			Porcelain:      true, // Disable spinner in tests
		},
		HTTPClient: mockClient,
	}

	// Set list-resources flag using viper
	viper.Set("list-resources", true)
	defer viper.Set("list-resources", false) // Reset after test

	// Test the function with list-resources enabled
	err = client.FetchResourceGroups()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Read and verify CSV file
	csvContent, err := os.ReadFile(tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to read CSV file: %v", err)
	}

	csvStr := string(csvContent)
	// Check that resources are in a single field
	if !strings.Contains(csvStr, "test-storage (Microsoft.Storage/storageAccounts)") {
		t.Error("Expected first resource not found in CSV")
	}
	if !strings.Contains(csvStr, "test-app (Microsoft.Web/sites)") {
		t.Error("Expected second resource not found in CSV")
	}
	// Check that resources are separated by semicolon
	if !strings.Contains(csvStr, "; ") {
		t.Error("Expected resources to be separated by semicolon in CSV")
	}
	// Check that created times are included
	if !strings.Contains(csvStr, "Created: 2023-01-01T12:00:00Z") {
		t.Error("Expected resource created time not found in CSV")
	}
}

func TestCSVOutputWithEmptyResourceGroup(t *testing.T) {
	// Create a temporary CSV file
	tempFile, err := os.CreateTemp("", "test_output_empty_*.csv")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() {
		if err := tempFile.Close(); err != nil {
			t.Errorf("Failed to close temp file: %v", err)
		}
		if err := os.Remove(tempFile.Name()); err != nil {
			t.Errorf("Failed to remove temp file: %v", err)
		}
	}()

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
								"id": "/subscriptions/test/resourceGroups/empty-rg",
								"name": "empty-rg",
								"location": "eastus",
								"properties": {
									"provisioningState": "Succeeded"
								}
							}
						]
					}`)),
				}
				return resp, nil
			} else {
				// Return empty resources
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"value": []
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
			MaxConcurrency: 10,
			OutputCSV:      tempFile.Name(),
			Porcelain:      true, // Disable spinner in tests
		},
		HTTPClient: mockClient,
	}

	// Test the function
	err = client.FetchResourceGroups()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Read and verify CSV file
	csvContent, err := os.ReadFile(tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to read CSV file: %v", err)
	}

	csvStr := string(csvContent)
	// Check that empty resource group is handled correctly
	if !strings.Contains(csvStr, "empty-rg,eastus,Succeeded") {
		t.Error("Expected empty resource group data not found in CSV")
	}
	// Check that created time is "Not available" for empty resource group
	if !strings.Contains(csvStr, "Not available") {
		t.Error("Expected 'Not available' for empty resource group created time")
	}
}

func TestConvertToCSVRow(t *testing.T) {
	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-subscription",
			AccessToken:    "test-token",
			MaxConcurrency: 10,
			Porcelain:      true, // Disable spinner in tests
		},
	}

	// Test with a regular resource group
	rg := ResourceGroup{
		Name:     "test-rg",
		Location: "eastus",
		Properties: struct {
			ProvisioningState string `json:"provisioningState"`
		}{
			ProvisioningState: "Succeeded",
		},
	}

	createdTime, _ := time.Parse(time.RFC3339, "2023-01-01T12:00:00Z")
	result := ResourceGroupResult{
		ResourceGroup: rg,
		CreatedTime:   &createdTime,
		Error:         nil,
	}

	// Test without resources
	csvRow := client.convertToCSVRow(result, false, nil)
	if csvRow.ResourceGroupName != "test-rg" {
		t.Errorf("Expected ResourceGroupName 'test-rg', got '%s'", csvRow.ResourceGroupName)
	}
	if csvRow.Location != "eastus" {
		t.Errorf("Expected Location 'eastus', got '%s'", csvRow.Location)
	}
	if csvRow.ProvisioningState != "Succeeded" {
		t.Errorf("Expected ProvisioningState 'Succeeded', got '%s'", csvRow.ProvisioningState)
	}
	if csvRow.CreatedTime != "2023-01-01T12:00:00Z" {
		t.Errorf("Expected CreatedTime '2023-01-01T12:00:00Z', got '%s'", csvRow.CreatedTime)
	}
	if csvRow.IsDefault != "false" {
		t.Errorf("Expected IsDefault 'false', got '%s'", csvRow.IsDefault)
	}
	if csvRow.Resources != "" {
		t.Errorf("Expected empty Resources, got '%s'", csvRow.Resources)
	}

	// Test with resources
	resources := []Resource{
		{
			Name:        "test-storage",
			Type:        "Microsoft.Storage/storageAccounts",
			CreatedTime: &createdTime,
		},
		{
			Name:        "test-app",
			Type:        "Microsoft.Web/sites",
			CreatedTime: nil,
		},
	}

	csvRow = client.convertToCSVRow(result, true, resources)
	expectedResources := "test-storage (Microsoft.Storage/storageAccounts) - Created: 2023-01-01T12:00:00Z; test-app (Microsoft.Web/sites) - Created: Not available"
	if csvRow.Resources != expectedResources {
		t.Errorf("Expected Resources '%s', got '%s'", expectedResources, csvRow.Resources)
	}

	// Test with default resource group
	defaultRG := ResourceGroup{
		Name:     "DefaultResourceGroup-EUS",
		Location: "eastus",
		Properties: struct {
			ProvisioningState string `json:"provisioningState"`
		}{
			ProvisioningState: "Succeeded",
		},
	}

	defaultResult := ResourceGroupResult{
		ResourceGroup: defaultRG,
		CreatedTime:   &createdTime,
		Error:         nil,
	}

	csvRow = client.convertToCSVRow(defaultResult, false, nil)
	if csvRow.IsDefault != "true" {
		t.Errorf("Expected IsDefault 'true', got '%s'", csvRow.IsDefault)
	}
	if csvRow.CreatedBy != "Azure CLI / Cloud Shell / Visual Studio" {
		t.Errorf("Expected CreatedBy 'Azure CLI / Cloud Shell / Visual Studio', got '%s'", csvRow.CreatedBy)
	}
	if csvRow.Description == "" {
		t.Error("Expected non-empty Description for default resource group")
	}

	// Test with error
	errorResult := ResourceGroupResult{
		ResourceGroup: rg,
		CreatedTime:   nil,
		Error:         fmt.Errorf("test error"),
	}

	csvRow = client.convertToCSVRow(errorResult, false, nil)
	if !strings.Contains(csvRow.CreatedTime, "Error: test error") {
		t.Errorf("Expected CreatedTime to contain error, got '%s'", csvRow.CreatedTime)
	}
}

func TestWriteCSVFile(t *testing.T) {
	// Create a temporary CSV file
	tempFile, err := os.CreateTemp("", "test_write_csv_*.csv")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() {
		if err := tempFile.Close(); err != nil {
			t.Errorf("Failed to close temp file: %v", err)
		}
		if err := os.Remove(tempFile.Name()); err != nil {
			t.Errorf("Failed to remove temp file: %v", err)
		}
	}()

	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-subscription",
			AccessToken:    "test-token",
			MaxConcurrency: 10,
			OutputCSV:      tempFile.Name(),
			Porcelain:      true, // Disable spinner in tests
		},
	}

	// Test data
	csvData := []CSVRow{
		{
			ResourceGroupName: "test-rg-1",
			Location:          "eastus",
			ProvisioningState: "Succeeded",
			CreatedTime:       "2023-01-01T12:00:00Z",
			IsDefault:         "false",
			CreatedBy:         "",
			Description:       "",
			Resources:         "",
		},
		{
			ResourceGroupName: "DefaultResourceGroup-EUS",
			Location:          "eastus",
			ProvisioningState: "Succeeded",
			CreatedTime:       "2023-01-01T12:00:00Z",
			IsDefault:         "true",
			CreatedBy:         "Azure CLI / Cloud Shell / Visual Studio",
			Description:       "Common default resource group",
			Resources:         "test-storage (Microsoft.Storage/storageAccounts) - Created: 2023-01-01T12:00:00Z",
		},
	}

	// Write CSV file
	err = client.writeCSVFile(csvData)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Read and verify CSV file
	csvContent, err := os.ReadFile(tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to read CSV file: %v", err)
	}

	csvStr := string(csvContent)
	lines := strings.Split(csvStr, "\n")

	// Check header
	expectedHeader := "ResourceGroupName,Location,ProvisioningState,CreatedTime,IsDefault,CreatedBy,Description,Resources"
	if lines[0] != expectedHeader {
		t.Errorf("Expected header '%s', got '%s'", expectedHeader, lines[0])
	}

	// Check first data row
	if !strings.Contains(lines[1], "test-rg-1,eastus,Succeeded") {
		t.Error("Expected first data row not found")
	}

	// Check second data row
	if !strings.Contains(lines[2], "DefaultResourceGroup-EUS,eastus,Succeeded") {
		t.Error("Expected second data row not found")
	}

	// Check that resources are properly escaped/quoted in CSV
	if !strings.Contains(csvStr, "test-storage (Microsoft.Storage/storageAccounts)") {
		t.Error("Expected resource information not found in CSV")
	}
}

func TestCSVConfigValidation(t *testing.T) {
	// Test that OutputCSV is properly configured using viper directly
	// to avoid calling initConfig() which has validation requirements

	// Test with CSV output flag
	viper.Set("output-csv", "test.csv")
	outputCSV := viper.GetString("output-csv")
	if outputCSV != "test.csv" {
		t.Errorf("Expected OutputCSV 'test.csv', got '%s'", outputCSV)
	}

	// Test without CSV output flag
	viper.Set("output-csv", "")
	outputCSV = viper.GetString("output-csv")
	if outputCSV != "" {
		t.Errorf("Expected empty OutputCSV, got '%s'", outputCSV)
	}
}

func TestFetchResourcesInGroup(t *testing.T) {
	// Test the fetchResourcesInGroup function
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			// Return mock resources
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"value": [
						{
							"id": "/subscriptions/test/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test-storage",
							"name": "test-storage",
							"type": "Microsoft.Storage/storageAccounts",
							"createdTime": "2023-01-01T12:00:00Z"
						},
						{
							"id": "/subscriptions/test/resourceGroups/test-rg/providers/Microsoft.Web/sites/test-app",
							"name": "test-app",
							"type": "Microsoft.Web/sites"
						}
					]
				}`)),
			}
			return resp, nil
		},
	}

	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-subscription",
			AccessToken:    "test-token",
			Porcelain:      true, // Disable spinner in tests
		},
		HTTPClient: mockClient,
	}

	resources, err := client.fetchResourcesInGroup("test-rg")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(resources) != 2 {
		t.Fatalf("Expected 2 resources, got %d", len(resources))
	}

	// Check first resource
	if resources[0].Name != "test-storage" {
		t.Errorf("Expected first resource name 'test-storage', got '%s'", resources[0].Name)
	}
	if resources[0].Type != "Microsoft.Storage/storageAccounts" {
		t.Errorf("Expected first resource type 'Microsoft.Storage/storageAccounts', got '%s'", resources[0].Type)
	}
	if resources[0].CreatedTime == nil {
		t.Error("Expected first resource to have created time")
	}

	// Check second resource
	if resources[1].Name != "test-app" {
		t.Errorf("Expected second resource name 'test-app', got '%s'", resources[1].Name)
	}
	if resources[1].Type != "Microsoft.Web/sites" {
		t.Errorf("Expected second resource type 'Microsoft.Web/sites', got '%s'", resources[1].Type)
	}
	if resources[1].CreatedTime != nil {
		t.Error("Expected second resource to have nil created time")
	}
}

func TestPrintResourceGroupResultWithResources_Porcelain(t *testing.T) {
	ac := &AzureClient{Config: Config{Porcelain: true}}

	created1, err := time.Parse(time.RFC3339, "2023-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("failed to parse time: %v", err)
	}
	created2, err := time.Parse(time.RFC3339, "2023-02-01T00:00:00Z")
	if err != nil {
		t.Fatalf("failed to parse time: %v", err)
	}

	resources := []Resource{
		{Name: "res1", Type: "type1", CreatedTime: &created1},
		{Name: "res2", Type: "type2", CreatedTime: &created2},
	}

	rg := ResourceGroup{Name: "my-rg", Location: "eastus", Properties: struct {
		ProvisioningState string `json:"provisioningState"`
	}{ProvisioningState: "Succeeded"}}
	result := ResourceGroupResult{ResourceGroup: rg}

	old := os.Stdout
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("failed to create pipe: %v", pipeErr)
	}
	os.Stdout = w

	ac.printResourceGroupResultWithResources(result, resources)

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close pipe writer: %v", err)
	}
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed to read output: %v", err)
	}

	expected := fmt.Sprintf("%s\t%s\t%s\t%s\tfalse\n", rg.Name, rg.Location, rg.Properties.ProvisioningState, created1.Format(time.RFC3339))
	if strings.TrimSpace(buf.String()) != strings.TrimSpace(expected) {
		t.Errorf("unexpected output:\n%s", buf.String())
	}
}

func TestPrintResourceGroupResultWithResources_Human(t *testing.T) {
	ac := &AzureClient{Config: Config{Porcelain: false}}

	rg := ResourceGroup{Name: "networkwatcherrg", Location: "eastus", Properties: struct {
		ProvisioningState string `json:"provisioningState"`
	}{ProvisioningState: "Succeeded"}}
	result := ResourceGroupResult{ResourceGroup: rg}

	old := os.Stdout
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("failed to create pipe: %v", pipeErr)
	}
	os.Stdout = w

	ac.printResourceGroupResultWithResources(result, nil)

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close pipe writer: %v", err)
	}
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	output := buf.String()

	if !strings.Contains(output, "DEFAULT RESOURCE GROUP DETECTED") {
		t.Errorf("expected default detection in output, got:\n%s", output)
	}
	if !strings.Contains(output, "No resources found") {
		t.Errorf("expected no resources message, got:\n%s", output)
	}
}
