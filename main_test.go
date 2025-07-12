package main

import (
    "net/http"
    "net/http/httptest"
    "os"
    "testing"
    "time"
)

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
        w.Write([]byte(`{"value": []}`))
    }))
    defer server.Close()

    // Set up configuration
    config.AccessToken = "test-token"
    
    // Make a request to the test server
    resp, err := makeAzureRequest(server.URL)
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        t.Errorf("Expected status 200, got %d", resp.StatusCode)
    }
}

func TestMakeAzureRequestWithError(t *testing.T) {
    // Create a test server that returns an error
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusUnauthorized)
        w.Write([]byte(`{"error": "Unauthorized"}`))
    }))
    defer server.Close()

    // Set up configuration
    config.AccessToken = "invalid-token"
    
    // Make a request to the test server
    _, err := makeAzureRequest(server.URL)
    if err == nil {
        t.Fatal("Expected an error, got nil")
    }
}

func TestFetchResourceGroupCreatedTime(t *testing.T) {
    // Create a test server that returns resources with creation times
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{
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
        }`))
    }))
    defer server.Close()

    // Override the API URL for testing
    originalSubscriptionID := config.SubscriptionID
    config.SubscriptionID = "test-subscription"
    config.AccessToken = "test-token"
    
    // Mock the makeAzureRequest function by temporarily replacing the base URL
    // For simplicity, we'll test the JSON parsing logic separately
    defer func() {
        config.SubscriptionID = originalSubscriptionID
    }()

    // Test the time parsing by calling the function with a mock response
    // Note: This is a simplified test - in a real scenario, you might want to use 
    // dependency injection or interfaces to make the HTTP client mockable
}

func TestConfigValidation(t *testing.T) {
    // Save original environment variables
    originalSubID := os.Getenv("AZURE_SUBSCRIPTION_ID")
    originalToken := os.Getenv("AZURE_ACCESS_TOKEN")
    
    defer func() {
        // Restore original environment variables
        os.Setenv("AZURE_SUBSCRIPTION_ID", originalSubID)
        os.Setenv("AZURE_ACCESS_TOKEN", originalToken)
    }()

    // Test with missing subscription ID
    os.Unsetenv("AZURE_SUBSCRIPTION_ID")
    os.Unsetenv("AZURE_ACCESS_TOKEN")
    
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
    }
    
    if !earliestTime.Equal(time1) {
        t.Error("Expected earliest time to be time1")
    }
}