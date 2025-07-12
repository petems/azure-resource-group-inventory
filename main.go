package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Pre-compiled regex patterns for better performance
var (
	defaultResourceGroupPattern = regexp.MustCompile(`^defaultresourcegroup-`)
	dynamicsPattern             = regexp.MustCompile(`^dynamicsdeployments$`)
	aksPattern                  = regexp.MustCompile(`^mc_.*_.*_.*$`)
	azureBackupPattern          = regexp.MustCompile(`^azurebackuprg`)
	networkWatcherPattern       = regexp.MustCompile(`^networkwatcherrg$`)
	databricksPattern           = regexp.MustCompile(`^databricks-rg`)
	microsoftNetworkPattern     = regexp.MustCompile(`^microsoft-network$`)
	logAnalyticsPattern         = regexp.MustCompile(`^loganalyticsdefaultresources$`)
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
	MaxConcurrency int
}

// Azure client struct
type AzureClient struct {
	Config     Config
	HTTPClient HTTPClient
}

// ResourceGroupResult holds the result of processing a resource group
type ResourceGroupResult struct {
	ResourceGroup ResourceGroup
	CreatedTime   *time.Time
	Error         error
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
	rootCmd.PersistentFlags().Int("max-concurrency", 10, "Maximum number of concurrent API calls (minimum: 1)")

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
	if err := viper.BindPFlag("max-concurrency", rootCmd.PersistentFlags().Lookup("max-concurrency")); err != nil {
		log.Fatalf("Failed to bind max-concurrency flag: %v", err)
	}
}

func initConfig() {
	// Read from environment variables
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	// Set defaults
	config.SubscriptionID = viper.GetString("subscription-id")
	config.AccessToken = viper.GetString("access-token")
	config.MaxConcurrency = viper.GetInt("max-concurrency")

	// If not provided via flags, try environment variables
	if config.SubscriptionID == "" {
		config.SubscriptionID = os.Getenv("AZURE_SUBSCRIPTION_ID")
	}
	if config.AccessToken == "" {
		config.AccessToken = os.Getenv("AZURE_ACCESS_TOKEN")
	}
	if config.MaxConcurrency == 0 {
		config.MaxConcurrency = 10
	}

	// Validate required configuration
	if config.SubscriptionID == "" {
		log.Fatal("Subscription ID is required. Set via --subscription-id flag or AZURE_SUBSCRIPTION_ID environment variable")
	}
	if config.AccessToken == "" {
		log.Fatal("Access token is required. Set via --access-token flag or AZURE_ACCESS_TOKEN environment variable")
	}
	
	// Validate concurrency configuration to prevent hanging
	config.MaxConcurrency = validateConcurrency(config.MaxConcurrency)

	// Initialize Azure client with optimized HTTP client
	azureClient = &AzureClient{
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

// DefaultResourceGroupInfo represents information about a default resource group
type DefaultResourceGroupInfo struct {
	IsDefault   bool
	CreatedBy   string
	Description string
}

// validateConcurrency ensures that the concurrency value is at least 1
// to prevent hanging due to zero-capacity channels
func validateConcurrency(concurrency int) int {
	if concurrency < 1 {
		log.Printf("Warning: Concurrency (%d) is less than 1, setting to 1 to prevent hanging", concurrency)
		return 1
	}
	return concurrency
}

// checkIfDefaultResourceGroup checks if a resource group name matches patterns of default resource groups
// Now uses pre-compiled regex patterns for better performance
func checkIfDefaultResourceGroup(name string) DefaultResourceGroupInfo {
	nameLower := strings.ToLower(name)

	// DefaultResourceGroup-XXX pattern
	if defaultResourceGroupPattern.MatchString(nameLower) {
		return DefaultResourceGroupInfo{
			IsDefault:   true,
			CreatedBy:   "Azure CLI / Cloud Shell / Visual Studio",
			Description: "Common default resource group created for the region, used by Azure CLI, Cloud Shell, and Visual Studio for resource deployment",
		}
	}

	// DynamicsDeployments pattern
	if dynamicsPattern.MatchString(nameLower) {
		return DefaultResourceGroupInfo{
			IsDefault:   true,
			CreatedBy:   "Microsoft Dynamics ERP",
			Description: "Automatically created for Microsoft Dynamics ERP non-production instances",
		}
	}

	// MC_* pattern for AKS
	if aksPattern.MatchString(nameLower) {
		return DefaultResourceGroupInfo{
			IsDefault:   true,
			CreatedBy:   "Azure Kubernetes Service (AKS)",
			Description: "Created when deploying an AKS cluster, contains infrastructure resources for the cluster",
		}
	}

	// AzureBackupRG* pattern
	if azureBackupPattern.MatchString(nameLower) {
		return DefaultResourceGroupInfo{
			IsDefault:   true,
			CreatedBy:   "Azure Backup",
			Description: "Created by Azure Backup service for backup operations",
		}
	}

	// NetworkWatcherRG pattern
	if networkWatcherPattern.MatchString(nameLower) {
		return DefaultResourceGroupInfo{
			IsDefault:   true,
			CreatedBy:   "Azure Network Watcher",
			Description: "Created by Azure Network Watcher service for network monitoring",
		}
	}

	// databricks-rg* pattern
	if databricksPattern.MatchString(nameLower) {
		return DefaultResourceGroupInfo{
			IsDefault:   true,
			CreatedBy:   "Azure Databricks",
			Description: "Created by Azure Databricks service for managed workspace resources",
		}
	}

	// microsoft-network pattern
	if microsoftNetworkPattern.MatchString(nameLower) {
		return DefaultResourceGroupInfo{
			IsDefault:   true,
			CreatedBy:   "Microsoft Networking Services",
			Description: "Used by Microsoft's networking services",
		}
	}

	// LogAnalyticsDefaultResources pattern
	if logAnalyticsPattern.MatchString(nameLower) {
		return DefaultResourceGroupInfo{
			IsDefault:   true,
			CreatedBy:   "Azure Log Analytics",
			Description: "Created by Azure Log Analytics service for default workspace resources",
		}
	}

	return DefaultResourceGroupInfo{
		IsDefault:   false,
		CreatedBy:   "",
		Description: "",
	}
}

func (ac *AzureClient) FetchResourceGroups() error {
	// Performance monitoring
	start := time.Now()
	defer func() {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		log.Printf("Operation completed in %v, Memory usage: %d KB", time.Since(start), m.Alloc/1024)
	}()

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

	// Process resource groups concurrently
	if listResources {
		ac.processResourceGroupsConcurrentlyWithResources(rgResponse.Value)
	} else {
		ac.processResourceGroupsConcurrently(rgResponse.Value)
	}

	return nil
}

// processResourceGroupsConcurrently processes resource groups concurrently for better performance
func (ac *AzureClient) processResourceGroupsConcurrently(resourceGroups []ResourceGroup) {
	var wg sync.WaitGroup
	results := make([]ResourceGroupResult, len(resourceGroups))

	// Ensure MaxConcurrency is at least 1 to prevent hanging
	maxConcurrency := validateConcurrency(ac.Config.MaxConcurrency)

	// Use a semaphore to limit concurrent goroutines
	semaphore := make(chan struct{}, maxConcurrency)

	// Start workers
	for i, rg := range resourceGroups {
		wg.Add(1)
		go func(i int, rg ResourceGroup) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			createdTime, err := ac.fetchResourceGroupCreatedTime(rg.Name)
			results[i] = ResourceGroupResult{
				ResourceGroup: rg,
				CreatedTime:   createdTime,
				Error:         err,
			}
		}(i, rg)
	}

	// Wait for all workers to complete
	wg.Wait()

	// Print all results
	for _, result := range results {
		ac.printResourceGroupResult(result, false)
	}
}

// processResourceGroupsConcurrentlyWithResources processes resource groups with detailed resource listing
func (ac *AzureClient) processResourceGroupsConcurrentlyWithResources(resourceGroups []ResourceGroup) {
	// For resource listing, we don't need concurrency for the initial setup
	// The concurrency will be handled by the resource listing itself
	for _, rg := range resourceGroups {
		result := ResourceGroupResult{
			ResourceGroup: rg,
			CreatedTime:   nil, // Will be handled in resource listing
			Error:         nil,
		}
		ac.printResourceGroupResult(result, true)
	}
}

// printResourceGroupResult prints the result of processing a resource group
func (ac *AzureClient) printResourceGroupResult(result ResourceGroupResult, listResources bool) {
	rg := result.ResourceGroup
	fmt.Printf("Resource Group: %s\n", rg.Name)
	fmt.Printf("  Location: %s\n", rg.Location)
	fmt.Printf("  Provisioning State: %s\n", rg.Properties.ProvisioningState)

	// Check if this is a default resource group
	defaultInfo := checkIfDefaultResourceGroup(rg.Name)
	if defaultInfo.IsDefault {
		fmt.Printf("  🔍 DEFAULT RESOURCE GROUP DETECTED\n")
		fmt.Printf("  📋 Created By: %s\n", defaultInfo.CreatedBy)
		fmt.Printf("  📝 Description: %s\n", defaultInfo.Description)
	}

	if listResources {
		// List all resources in this resource group
		if err := ac.listResourcesInGroup(rg.Name); err != nil {
			fmt.Printf("  Error listing resources: %v\n", err)
		}
	} else {
		// Just show the creation time
		if result.Error != nil {
			fmt.Printf("  Created Time: Error fetching (%v)\n", result.Error)
		} else if result.CreatedTime != nil {
			fmt.Printf("  Created Time: %s\n", result.CreatedTime.Format(time.RFC3339))
		} else {
			fmt.Printf("  Created Time: Not available\n")
		}
	}

	fmt.Println()
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
