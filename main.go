package main

import (
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"
    "strings"
    "time"

    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

// Azure API structures
type ResourceGroup struct {
    ID       string `json:"id"`
    Name     string `json:"name"`
    Location string `json:"location"`
    Properties struct {
        ProvisioningState string `json:"provisioningState"`
    } `json:"properties"`
}

type ResourceGroupsResponse struct {
    Value []ResourceGroup `json:"value"`
}

type Resource struct {
    ID         string    `json:"id"`
    Name       string    `json:"name"`
    Type       string    `json:"type"`
    CreatedTime *time.Time `json:"createdTime,omitempty"`
}

type ResourcesResponse struct {
    Value []Resource `json:"value"`
}

// CLI configuration
type Config struct {
    SubscriptionID string
    AccessToken    string
}

var config Config

// Root command
var rootCmd = &cobra.Command{
    Use:   "azure-rg-cli",
    Short: "A CLI tool to fetch Azure resource groups and their creation times",
    Long: `A command-line tool that fetches all Azure resource groups from a subscription
and retrieves their creation times using the Azure Management API.`,
    Run: func(cmd *cobra.Command, args []string) {
        if err := fetchResourceGroups(); err != nil {
            log.Fatalf("Error fetching resource groups: %v", err)
        }
    },
}

func init() {
    cobra.OnInitialize(initConfig)
    
    // Add flags
    rootCmd.PersistentFlags().String("subscription-id", "", "Azure subscription ID")
    rootCmd.PersistentFlags().String("access-token", "", "Azure access token")
    
    // Bind flags to viper
    viper.BindPFlag("subscription-id", rootCmd.PersistentFlags().Lookup("subscription-id"))
    viper.BindPFlag("access-token", rootCmd.PersistentFlags().Lookup("access-token"))
}

func initConfig() {
    // Read from environment variables
    viper.AutomaticEnv()
    viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
    
    // Set defaults
    config.SubscriptionID = viper.GetString("subscription-id")
    config.AccessToken = viper.GetString("access-token")
    
    // If not provided via flags, try environment variables
    if config.SubscriptionID == "" {
        config.SubscriptionID = os.Getenv("AZURE_SUBSCRIPTION_ID")
    }
    if config.AccessToken == "" {
        config.AccessToken = os.Getenv("AZURE_ACCESS_TOKEN")
    }
    
    // Validate required configuration
    if config.SubscriptionID == "" {
        log.Fatal("Subscription ID is required. Set via --subscription-id flag or AZURE_SUBSCRIPTION_ID environment variable")
    }
    if config.AccessToken == "" {
        log.Fatal("Access token is required. Set via --access-token flag or AZURE_ACCESS_TOKEN environment variable")
    }
}

func makeAzureRequest(url string) (*http.Response, error) {
    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }
    
    req.Header.Set("Authorization", "Bearer "+config.AccessToken)
    req.Header.Set("Content-Type", "application/json")
    
    client := &http.Client{Timeout: 30 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("failed to make request: %w", err)
    }
    
    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        resp.Body.Close()
        return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
    }
    
    return resp, nil
}

func fetchResourceGroups() error {
    fmt.Println("Fetching resource groups...")
    
    // Fetch all resource groups
    url := fmt.Sprintf("https://management.azure.com/subscriptions/%s/resourcegroups?api-version=2021-04-01", config.SubscriptionID)
    
    resp, err := makeAzureRequest(url)
    if err != nil {
        return fmt.Errorf("failed to fetch resource groups: %w", err)
    }
    defer resp.Body.Close()
    
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return fmt.Errorf("failed to read response body: %w", err)
    }
    
    var rgResponse ResourceGroupsResponse
    if err := json.Unmarshal(body, &rgResponse); err != nil {
        return fmt.Errorf("failed to parse response: %w", err)
    }
    
    fmt.Printf("Found %d resource groups:\n\n", len(rgResponse.Value))
    
    // For each resource group, fetch its creation time
    for _, rg := range rgResponse.Value {
        fmt.Printf("Resource Group: %s\n", rg.Name)
        fmt.Printf("  Location: %s\n", rg.Location)
        fmt.Printf("  Provisioning State: %s\n", rg.Properties.ProvisioningState)
        
        createdTime, err := fetchResourceGroupCreatedTime(rg.Name)
        if err != nil {
            fmt.Printf("  Created Time: Error fetching (%v)\n", err)
        } else if createdTime != nil {
            fmt.Printf("  Created Time: %s\n", createdTime.Format(time.RFC3339))
        } else {
            fmt.Printf("  Created Time: Not available\n")
        }
        
        fmt.Println()
    }
    
    return nil
}

func fetchResourceGroupCreatedTime(resourceGroupName string) (*time.Time, error) {
    url := fmt.Sprintf("https://management.azure.com/subscriptions/%s/resourceGroups/%s/resources?$expand=createdTime&api-version=2019-10-01", 
        config.SubscriptionID, resourceGroupName)
    
    resp, err := makeAzureRequest(url)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch resources: %w", err)
    }
    defer resp.Body.Close()
    
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("failed to read response body: %w", err)
    }
    
    var resourcesResponse ResourcesResponse
    if err := json.Unmarshal(body, &resourcesResponse); err != nil {
        return nil, fmt.Errorf("failed to parse response: %w", err)
    }
    
    // Find the earliest created time among all resources in the resource group
    var earliestTime *time.Time
    for _, resource := range resourcesResponse.Value {
        if resource.CreatedTime != nil {
            if earliestTime == nil || resource.CreatedTime.Before(*earliestTime) {
                earliestTime = resource.CreatedTime
            }
        }
    }
    
    return earliestTime, nil
}

func main() {
    if err := rootCmd.Execute(); err != nil {
        log.Fatal(err)
    }
}