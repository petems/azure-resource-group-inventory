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

// HTTP client interface for testing
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Azure API structures
type ResourceGroup struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Location   string `json:"location"`
	Properties struct {
		ProvisioningState string `json:"provisioningState"`
	} `json:"properties"`
}

type ResourceGroupsResponse struct {
	Value []ResourceGroup `json:"value"`
}

type Resource struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Type        string     `json:"type"`
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

// Azure client struct
type AzureClient struct {
	Config     Config
	HTTPClient HTTPClient
}

var config Config
var azureClient *AzureClient

// Root command
var rootCmd = &cobra.Command{
	Use:   "azure-rg-cli",
	Short: "A CLI tool to fetch Azure resource groups and their creation times",
	Long: `A command-line tool that fetches all Azure resource groups from a subscription
and retrieves their creation times using the Azure Management API.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := azureClient.FetchResourceGroups(); err != nil {
			log.Fatalf("Error fetching resource groups: %v", err)
		}
	},
}

func init() {
	cobra.OnInitialize(initConfig)

	// Add flags
	rootCmd.PersistentFlags().String("subscription-id", "", "Azure subscription ID")
	rootCmd.PersistentFlags().String("access-token", "", "Azure access token")
	rootCmd.PersistentFlags().Bool("list-resources", false, "List all resources in each resource group with their creation times")

	// Bind flags to viper
	if err := viper.BindPFlag("subscription-id", rootCmd.PersistentFlags().Lookup("subscription-id")); err != nil {
		log.Fatalf("Failed to bind subscription-id flag: %v", err)
	}
	if err := viper.BindPFlag("access-token", rootCmd.PersistentFlags().Lookup("access-token")); err != nil {
		log.Fatalf("Failed to bind access-token flag: %v", err)
	}
	if err := viper.BindPFlag("list-resources", rootCmd.PersistentFlags().Lookup("list-resources")); err != nil {
		log.Fatalf("Failed to bind list-resources flag: %v", err)
	}
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

	// Initialize Azure client
	azureClient = &AzureClient{
		Config:     config,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (ac *AzureClient) makeAzureRequest(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+ac.Config.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ac.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if err := resp.Body.Close(); err != nil {
			log.Printf("Warning: failed to close response body: %v", err)
		}
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}

func (ac *AzureClient) FetchResourceGroups() error {
	fmt.Println("Fetching resource groups...")

	// Fetch all resource groups
	url := fmt.Sprintf("https://management.azure.com/subscriptions/%s/resourcegroups?api-version=2021-04-01", ac.Config.SubscriptionID)

	resp, err := ac.makeAzureRequest(url)
	if err != nil {
		return fmt.Errorf("failed to fetch resource groups: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Warning: failed to close response body: %v", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var rgResponse ResourceGroupsResponse
	if err := json.Unmarshal(body, &rgResponse); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("Found %d resource groups:\n\n", len(rgResponse.Value))

	// Check if we should list resources
	listResources := viper.GetBool("list-resources")

	// For each resource group, fetch its creation time
	for _, rg := range rgResponse.Value {
		fmt.Printf("Resource Group: %s\n", rg.Name)
		fmt.Printf("  Location: %s\n", rg.Location)
		fmt.Printf("  Provisioning State: %s\n", rg.Properties.ProvisioningState)

		if listResources {
			// List all resources in this resource group
			if err := ac.listResourcesInGroup(rg.Name); err != nil {
				fmt.Printf("  Error listing resources: %v\n", err)
			}
		} else {
			// Just show the earliest creation time
			createdTime, err := ac.fetchResourceGroupCreatedTime(rg.Name)
			if err != nil {
				fmt.Printf("  Created Time: Error fetching (%v)\n", err)
			} else if createdTime != nil {
				fmt.Printf("  Created Time: %s\n", createdTime.Format(time.RFC3339))
			} else {
				fmt.Printf("  Created Time: Not available\n")
			}
		}

		fmt.Println()
	}

	return nil
}

func (ac *AzureClient) fetchResourceGroupCreatedTime(resourceGroupName string) (*time.Time, error) {
	url := fmt.Sprintf("https://management.azure.com/subscriptions/%s/resourceGroups/%s/resources?$expand=createdTime&api-version=2019-10-01",
		ac.Config.SubscriptionID, resourceGroupName)

	resp, err := ac.makeAzureRequest(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch resources: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Warning: failed to close response body: %v", err)
		}
	}()

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

func (ac *AzureClient) listResourcesInGroup(resourceGroupName string) error {
	url := fmt.Sprintf("https://management.azure.com/subscriptions/%s/resourceGroups/%s/resources?$expand=createdTime&api-version=2019-10-01",
		ac.Config.SubscriptionID, resourceGroupName)

	resp, err := ac.makeAzureRequest(url)
	if err != nil {
		return fmt.Errorf("failed to fetch resources: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Warning: failed to close response body: %v", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var resourcesResponse ResourcesResponse
	if err := json.Unmarshal(body, &resourcesResponse); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if len(resourcesResponse.Value) == 0 {
		fmt.Printf("  No resources found in this resource group\n")
		return nil
	}

	fmt.Printf("  Resources (%d):\n", len(resourcesResponse.Value))
	for _, resource := range resourcesResponse.Value {
		fmt.Printf("    - %s (%s)\n", resource.Name, resource.Type)
		if resource.CreatedTime != nil {
			fmt.Printf("      Created: %s\n", resource.CreatedTime.Format(time.RFC3339))
		} else {
			fmt.Printf("      Created: Not available\n")
		}
	}

	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
