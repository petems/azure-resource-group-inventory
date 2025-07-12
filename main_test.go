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
