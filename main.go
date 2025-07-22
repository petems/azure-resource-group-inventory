package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
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
	defaultServicePattern       = regexp.MustCompile(`^default-[a-z0-9]+(-[a-z0-9]+)*$`)
	cloudShellStoragePattern    = regexp.MustCompile(`^cloud-shell-storage-[a-z0-9]+$`)
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

// Storage Account structures
type StorageAccount struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Location    string     `json:"location"`
	Type        string     `json:"type"`
	CreatedTime *time.Time `json:"createdTime,omitempty"`
	Properties  struct {
		ProvisioningState string     `json:"provisioningState"`
		CreationTime      *time.Time `json:"creationTime,omitempty"`
		PrimaryEndpoints  struct {
			Blob  string `json:"blob"`
			Queue string `json:"queue"`
			Table string `json:"table"`
			File  string `json:"file"`
		} `json:"primaryEndpoints"`
		AccountType string `json:"accountType"`
	} `json:"properties"`
	Tags map[string]string `json:"tags"`
}

type StorageAccountResponse struct {
	Value []StorageAccount `json:"value"`
}

type StorageAccountResult struct {
	StorageAccount StorageAccount
	CreatedTime    *time.Time
	Error          error
}

// CLI configuration
type Config struct {
	SubscriptionID string
	AccessToken    string
	MaxConcurrency int
	OutputCSV      string
	Porcelain      bool
}

// Spinner represents a simple text spinner for CLI feedback
type Spinner struct {
	message string
	active  bool
	done    chan bool
}

// NewSpinner creates a new spinner with the given message
func NewSpinner(message string) *Spinner {
	return &Spinner{
		message: message,
		done:    make(chan bool),
	}
}

// Start begins the spinner animation
func (s *Spinner) Start() {
	s.active = true
	go func() {
		frames := []string{"|", "/", "-", "\\"}
		i := 0
		for {
			select {
			case <-s.done:
				return
			default:
				if s.active {
					fmt.Printf("\r%s %s", frames[i], s.message)
					i = (i + 1) % len(frames)
					time.Sleep(100 * time.Millisecond)
				}
			}
		}
	}()
}

// Stop terminates the spinner and clears the line
func (s *Spinner) Stop() {
	s.active = false
	s.done <- true
	close(s.done)
	fmt.Print("\r\033[K") // Clear the line
}

// CommandProcessor interface for different Azure resource types
type CommandProcessor interface {
	FetchData() error
	GetName() string
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

// ResourceGroupProcessor implements CommandProcessor for resource groups
type ResourceGroupProcessor struct {
	client *AzureClient
}

func NewResourceGroupProcessor(client *AzureClient) *ResourceGroupProcessor {
	return &ResourceGroupProcessor{client: client}
}

func (rgp *ResourceGroupProcessor) FetchData() error {
	return rgp.client.FetchResourceGroups()
}

func (rgp *ResourceGroupProcessor) GetName() string {
	return "resource groups"
}

// StorageAccountProcessor implements CommandProcessor for storage accounts
type StorageAccountProcessor struct {
	client *AzureClient
}

func NewStorageAccountProcessor(client *AzureClient) *StorageAccountProcessor {
	return &StorageAccountProcessor{client: client}
}

func (sap *StorageAccountProcessor) FetchData() error {
	return sap.client.FetchStorageAccounts()
}

func (sap *StorageAccountProcessor) GetName() string {
	return "storage accounts"
}

/*
// Example: How to add a new command type
// 1. Create a new processor
type VirtualMachineProcessor struct {
	client *AzureClient
}

func NewVirtualMachineProcessor(client *AzureClient) *VirtualMachineProcessor {
	return &VirtualMachineProcessor{client: client}
}

func (vmp *VirtualMachineProcessor) FetchData() error {
	return vmp.client.FetchVirtualMachines()
}

func (vmp *VirtualMachineProcessor) GetName() string {
	return "virtual machines"
}

// 2. Add the command in init()
var virtualMachinesCmd = &cobra.Command{
	Use:   "virtual-machines",
	Short: "List all virtual machines with their details",
	Run: func(cmd *cobra.Command, args []string) {
		runner := NewCommandRunner(azureClient)
		processor := NewVirtualMachineProcessor(azureClient)
		if err := runner.RunCommand(processor); err != nil {
			log.Fatalf("Error fetching virtual machines: %v", err)
		}
	},
}

// 3. Add to root command in init()
rootCmd.AddCommand(virtualMachinesCmd)
*/

var config Config
var azureClient *AzureClient

// CommandRunner handles the execution of different command types
type CommandRunner struct {
	client *AzureClient
}

func NewCommandRunner(client *AzureClient) *CommandRunner {
	return &CommandRunner{client: client}
}

func (cr *CommandRunner) RunCommand(processor CommandProcessor) error {
	if !cr.client.Config.Porcelain {
		fmt.Printf("Fetching %s...\n", processor.GetName())
	}

	return processor.FetchData()
}

// Root command
var rootCmd = &cobra.Command{
	Use:   "azrginventory",
	Short: "A CLI tool to get a full inventory of Azure resource groups and their creation times",
	Long: `A command-line tool that fetches all Azure resource groups from a subscription
and retrieves their creation times (based on the earliest resource in the group) using the Azure Management API.`,
	Run: func(cmd *cobra.Command, args []string) {
		runner := NewCommandRunner(azureClient)
		processor := NewResourceGroupProcessor(azureClient)
		if err := runner.RunCommand(processor); err != nil {
			log.Fatalf("Error fetching resource groups: %v", err)
		}
	},
}

// Storage accounts command
var storageAccountsCmd = &cobra.Command{
	Use:   "storage-accounts",
	Short: "List all storage accounts with their creation times and identify limits",
	Long: `A command-line tool that fetches all Azure storage accounts from a subscription,
retrieves their creation times, and identifies accounts approaching location-based limits.`,
	Run: func(cmd *cobra.Command, args []string) {
		runner := NewCommandRunner(azureClient)
		processor := NewStorageAccountProcessor(azureClient)
		if err := runner.RunCommand(processor); err != nil {
			log.Fatalf("Error fetching storage accounts: %v", err)
		}
	},
}

func init() {
	cobra.OnInitialize(initConfig)

	// Initialize random seed for jitter in rate limiting (no longer needed in Go 1.20+)
	// Add subcommands
	rootCmd.AddCommand(storageAccountsCmd)

	// Add flags
	rootCmd.PersistentFlags().String("subscription-id", "", "Azure subscription ID")
	rootCmd.PersistentFlags().String("access-token", "", "Azure access token")
	rootCmd.PersistentFlags().Bool("list-resources", false, "List all resources in each resource group with their creation times")
	rootCmd.PersistentFlags().Int("max-concurrency", 10, "Maximum number of concurrent API calls (minimum: 1)")
	rootCmd.PersistentFlags().String("output-csv", "", "Output results to CSV file (specify file path)")
	rootCmd.PersistentFlags().Bool("porcelain", false, "Output results in a machine-readable format optimized for scripts (tab-separated values, no spinner)")

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
	if err := viper.BindPFlag("output-csv", rootCmd.PersistentFlags().Lookup("output-csv")); err != nil {
		log.Fatalf("Failed to bind output-csv flag: %v", err)
	}
	if err := viper.BindPFlag("porcelain", rootCmd.PersistentFlags().Lookup("porcelain")); err != nil {
		log.Fatalf("Failed to bind porcelain flag: %v", err)
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
	config.OutputCSV = viper.GetString("output-csv")
	config.Porcelain = viper.GetBool("porcelain")

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
	return ac.makeAzureRequestWithRetry(url, 0)
}

func (ac *AzureClient) makeAzureRequestWithRetry(url string, attempt int) (*http.Response, error) {
	const maxRetries = 5
	const baseDelay = 1 * time.Second

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

	// Handle rate limiting (429) with exponential backoff
	if resp.StatusCode == http.StatusTooManyRequests {
		if attempt >= maxRetries {
			body, _ := io.ReadAll(resp.Body)
			if err := resp.Body.Close(); err != nil {
				log.Printf("Warning: failed to close response body: %v", err)
			}
			return nil, fmt.Errorf("API request failed with status %d after %d retries: %s", resp.StatusCode, maxRetries, string(body))
		}

		// Calculate delay with exponential backoff and jitter
		delay := baseDelay * time.Duration(1<<attempt)              // Exponential backoff: 1s, 2s, 4s, 8s, 16s
		jitter := time.Duration(rand.Intn(1000)) * time.Millisecond // Add up to 1s of jitter
		totalDelay := delay + jitter

		if !ac.Config.Porcelain {
			log.Printf("Rate limited (429), retrying in %v (attempt %d/%d)", totalDelay, attempt+1, maxRetries)
		}

		// Close the response body before retrying
		if err := resp.Body.Close(); err != nil {
			log.Printf("Warning: failed to close response body: %v", err)
		}

		time.Sleep(totalDelay)
		return ac.makeAzureRequestWithRetry(url, attempt+1)
	}

	// Handle other non-200 status codes
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

	// Default-ServiceName-Region pattern (e.g., Default-Storage-EastUS, Default-EventHub-EastUS)
	if defaultServicePattern.MatchString(nameLower) {
		return DefaultResourceGroupInfo{
			IsDefault:   true,
			CreatedBy:   "Azure Services",
			Description: "Default resource group created by Azure services for regional deployments",
		}
	}

	// cloud-shell-storage-region pattern (e.g., cloud-shell-storage-eastus)
	if cloudShellStoragePattern.MatchString(nameLower) {
		return DefaultResourceGroupInfo{
			IsDefault:   true,
			CreatedBy:   "Azure Cloud Shell",
			Description: "Default storage resource group created by Azure Cloud Shell for persistent storage",
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

	if !ac.Config.Porcelain {
		fmt.Println("Fetching resource groups...")
	}

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

	if ac.Config.Porcelain {
		// Print header for porcelain mode
		fmt.Printf("NAME\tLOCATION\tPROVISIONING_STATE\tCREATED_TIME\tIS_DEFAULT\n")
	} else {
		fmt.Printf("Found %d resource groups:\n\n", len(rgResponse.Value))
	}

	// Check if we should list resources
	listResources := viper.GetBool("list-resources")

	// Check if CSV output is enabled
	outputCSV := ac.Config.OutputCSV != ""

	var csvData []CSVRow
	if outputCSV {
		csvData = make([]CSVRow, 0, len(rgResponse.Value))
	}

	// Process resource groups concurrently
	if listResources {
		if outputCSV {
			csvData = ac.processResourceGroupsConcurrentlyWithResourcesCSV(rgResponse.Value)
		} else {
			ac.processResourceGroupsConcurrentlyWithResources(rgResponse.Value)
		}
	} else {
		if outputCSV {
			csvData = ac.processResourceGroupsConcurrentlyCSV(rgResponse.Value)
		} else {
			ac.processResourceGroupsConcurrently(rgResponse.Value)
		}
	}

	// Write CSV data if output is enabled
	if outputCSV {
		if err := ac.writeCSVFile(csvData); err != nil {
			return fmt.Errorf("failed to write CSV file: %w", err)
		}
		if !ac.Config.Porcelain {
			fmt.Printf("CSV output written to: %s\n", ac.Config.OutputCSV)
		}
	}

	return nil
}

func (ac *AzureClient) FetchStorageAccounts() error {
	url := fmt.Sprintf("https://management.azure.com/subscriptions/%s/providers/Microsoft.Storage/storageAccounts?$expand=createdTime&api-version=2021-09-01",
		ac.Config.SubscriptionID)

	resp, err := ac.makeAzureRequest(url)
	if err != nil {
		return fmt.Errorf("failed to fetch storage accounts: %w", err)
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

	var storageAccountsResponse StorageAccountResponse
	if err := json.Unmarshal(body, &storageAccountsResponse); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if len(storageAccountsResponse.Value) == 0 {
		fmt.Println("No storage accounts found in this subscription.")
		return nil
	}

	if !ac.Config.Porcelain {
		fmt.Printf("Found %d storage accounts:\n\n", len(storageAccountsResponse.Value))
	}

	// Check if CSV output is enabled
	outputCSV := ac.Config.OutputCSV != ""

	var csvData []StorageAccountCSVRow
	if outputCSV {
		csvData = make([]StorageAccountCSVRow, 0, len(storageAccountsResponse.Value))
	}

	// Process storage accounts concurrently
	if outputCSV {
		csvData = ac.processStorageAccountsConcurrentlyCSV(storageAccountsResponse.Value)
	} else {
		ac.processStorageAccountsConcurrently(storageAccountsResponse.Value)
	}

	// Write CSV data if output is enabled
	if outputCSV {
		if err := ac.writeStorageAccountCSVFile(csvData); err != nil {
			return fmt.Errorf("failed to write CSV file: %w", err)
		}
		if !ac.Config.Porcelain {
			fmt.Printf("CSV output written to: %s\n", ac.Config.OutputCSV)
		}
	}

	return nil
}

// processStorageAccountsConcurrently processes storage accounts concurrently for better performance
func (ac *AzureClient) processStorageAccountsConcurrently(storageAccounts []StorageAccount) {
	// Since we now get creation time from the initial API call, we can process synchronously
	results := make([]StorageAccountResult, len(storageAccounts))

	// Start spinner if not in porcelain mode
	var spinner *Spinner
	if !ac.Config.Porcelain {
		spinner = NewSpinner("Processing storage accounts...")
		spinner.Start()
	}

	// Process storage accounts (no additional API calls needed)
	for i, sa := range storageAccounts {
		// Use creation time from properties if available, otherwise from root level
		createdTime := sa.Properties.CreationTime
		if createdTime == nil {
			createdTime = sa.CreatedTime
		}

		results[i] = StorageAccountResult{
			StorageAccount: sa,
			CreatedTime:    createdTime,
			Error:          nil,
		}
	}

	// Stop spinner if it was started
	if spinner != nil {
		spinner.Stop()
	}

	// Print results and analyze limits
	ac.printStorageAccountResults(results)
}

// processStorageAccountsConcurrentlyCSV processes storage accounts concurrently and returns CSV data
func (ac *AzureClient) processStorageAccountsConcurrentlyCSV(storageAccounts []StorageAccount) []StorageAccountCSVRow {
	// Since we now get creation time from the initial API call, we can process synchronously
	results := make([]StorageAccountResult, len(storageAccounts))

	// Start spinner if not in porcelain mode
	var spinner *Spinner
	if !ac.Config.Porcelain {
		spinner = NewSpinner("Processing storage accounts for CSV...")
		spinner.Start()
	}

	// Process storage accounts (no additional API calls needed)
	for i, sa := range storageAccounts {
		// Use creation time from properties if available, otherwise from root level
		createdTime := sa.Properties.CreationTime
		if createdTime == nil {
			createdTime = sa.CreatedTime
		}

		results[i] = StorageAccountResult{
			StorageAccount: sa,
			CreatedTime:    createdTime,
			Error:          nil,
		}
	}

	// Stop spinner if it was started
	if spinner != nil {
		spinner.Stop()
	}

	// Convert results to CSV format
	csvData := make([]StorageAccountCSVRow, 0, len(results))
	for _, result := range results {
		csvRow := ac.convertStorageAccountToCSVRow(result)
		csvData = append(csvData, csvRow)
		// Also print to console
		ac.printStorageAccountResult(result)
	}

	return csvData
}

// convertStorageAccountToCSVRow converts a StorageAccountResult to a StorageAccountCSVRow
func (ac *AzureClient) convertStorageAccountToCSVRow(result StorageAccountResult) StorageAccountCSVRow {
	sa := result.StorageAccount

	// Format created time
	createdTimeStr := "Not available"
	if result.Error != nil {
		createdTimeStr = "Error: " + result.Error.Error()
	} else if result.CreatedTime != nil {
		createdTimeStr = result.CreatedTime.Format(time.RFC3339)
	}

	// Format account type - try to get it from SKU if not available in properties
	accountType := sa.Properties.AccountType
	if accountType == "" || accountType == "Unknown" {
		// Try to infer from the storage account kind or other properties
		if sa.Type == "Microsoft.Storage/storageAccounts" {
			accountType = "Standard_LRS" // Default assumption
		} else {
			accountType = "Unknown"
		}
	}

	// Extract resource group from ID
	resourceGroup := extractResourceGroupFromID(sa.ID)

	// Format error
	errorStr := ""
	if result.Error != nil {
		errorStr = result.Error.Error()
	}

	return StorageAccountCSVRow{
		StorageAccountName: sa.Name,
		Location:           sa.Location,
		AccountType:        accountType,
		ProvisioningState:  sa.Properties.ProvisioningState,
		CreatedTime:        createdTimeStr,
		ResourceGroup:      resourceGroup,
		BlobEndpoint:       sa.Properties.PrimaryEndpoints.Blob,
		QueueEndpoint:      sa.Properties.PrimaryEndpoints.Queue,
		TableEndpoint:      sa.Properties.PrimaryEndpoints.Table,
		FileEndpoint:       sa.Properties.PrimaryEndpoints.File,
		Error:              errorStr,
	}
}

// printStorageAccountResult prints a single storage account result
func (ac *AzureClient) printStorageAccountResult(result StorageAccountResult) {
	if ac.Config.Porcelain {
		// Porcelain mode: compact, single-line format for scripts
		createdTime := ""
		if result.Error != nil {
			createdTime = "ERROR"
		} else if result.CreatedTime != nil {
			createdTime = result.CreatedTime.Format(time.RFC3339)
		} else {
			createdTime = "Not available"
		}

		accountType := result.StorageAccount.Properties.AccountType
		if accountType == "" || accountType == "Unknown" {
			accountType = "Standard_LRS"
		}

		fmt.Printf("%s\t%s\t%s\t%s\t%s\n",
			result.StorageAccount.Name,
			result.StorageAccount.Location,
			accountType,
			result.StorageAccount.Properties.ProvisioningState,
			createdTime)
	} else {
		// Human-readable format
		sa := result.StorageAccount
		createdTime := "Not available"
		if result.Error != nil {
			createdTime = "Error fetching (" + result.Error.Error() + ")"
		} else if result.CreatedTime != nil {
			createdTime = result.CreatedTime.Format(time.RFC3339)
		}

		accountType := sa.Properties.AccountType
		if accountType == "" || accountType == "Unknown" {
			accountType = "Standard_LRS (inferred)"
		}

		fmt.Printf("\nStorage Account: %s\n", sa.Name)
		fmt.Printf("  Location: %s\n", sa.Location)
		fmt.Printf("  Account Type: %s\n", accountType)
		fmt.Printf("  Provisioning State: %s\n", sa.Properties.ProvisioningState)
		fmt.Printf("  Created: %s\n", createdTime)

		// Show endpoint information
		if sa.Properties.PrimaryEndpoints.Blob != "" {
			fmt.Printf("  Blob Endpoint: %s\n", sa.Properties.PrimaryEndpoints.Blob)
		}
		if sa.Properties.PrimaryEndpoints.Queue != "" {
			fmt.Printf("  Queue Endpoint: %s\n", sa.Properties.PrimaryEndpoints.Queue)
		}
		if sa.Properties.PrimaryEndpoints.Table != "" {
			fmt.Printf("  Table Endpoint: %s\n", sa.Properties.PrimaryEndpoints.Table)
		}
		if sa.Properties.PrimaryEndpoints.File != "" {
			fmt.Printf("  File Endpoint: %s\n", sa.Properties.PrimaryEndpoints.File)
		}
	}
}

func (ac *AzureClient) printStorageAccountResults(results []StorageAccountResult) {
	// Group storage accounts by location and account type
	locationCounts := make(map[string]map[string]int)
	locationAccounts := make(map[string][]StorageAccountResult)
	standardDNSAccounts := make(map[string][]StorageAccountResult)

	for _, result := range results {
		if result.Error != nil {
			fmt.Printf("Error processing storage account %s: %v\n", result.StorageAccount.Name, result.Error)
			continue
		}

		location := result.StorageAccount.Location
		accountType := result.StorageAccount.Properties.AccountType
		if accountType == "" {
			accountType = "Unknown"
		}

		// Initialize maps if needed
		if locationCounts[location] == nil {
			locationCounts[location] = make(map[string]int)
		}
		if locationAccounts[location] == nil {
			locationAccounts[location] = make([]StorageAccountResult, 0)
		}

		locationCounts[location][accountType]++
		locationAccounts[location] = append(locationAccounts[location], result)

		// Track Standard DNS accounts specifically (these are the ones causing the limit issue)
		if isStandardDNSAccount(accountType) {
			if standardDNSAccounts[location] == nil {
				standardDNSAccounts[location] = make([]StorageAccountResult, 0)
			}
			standardDNSAccounts[location] = append(standardDNSAccounts[location], result)
		}
	}

	// Print summary by location
	fmt.Println("=== STORAGE ACCOUNT SUMMARY BY LOCATION ===")
	for location, accountTypes := range locationCounts {
		fmt.Printf("\nLocation: %s\n", location)
		totalInLocation := 0
		for accountType, count := range accountTypes {
			fmt.Printf("  %s: %d accounts\n", accountType, count)
			totalInLocation += count
		}
		fmt.Printf("  Total: %d accounts\n", totalInLocation)

		// Check for limits (Azure allows 250 storage accounts per subscription per region)
		if totalInLocation >= STORAGE_ACCOUNT_WARNING_THRESHOLD {
			fmt.Printf("  ⚠️  WARNING: Approaching limit of %d storage accounts per region!\n", STORAGE_ACCOUNT_LIMIT)
		}
		if totalInLocation >= STORAGE_ACCOUNT_LIMIT {
			fmt.Printf("  🚨 ERROR: At limit of %d storage accounts per region!\n", STORAGE_ACCOUNT_LIMIT)
		}
	}

	// Print Standard DNS endpoint analysis
	fmt.Println("\n=== STANDARD DNS ENDPOINT ANALYSIS ===")
	for location, accounts := range standardDNSAccounts {
		count := len(accounts)
		fmt.Printf("\nLocation: %s - Standard DNS accounts: %d\n", location, count)

		if count >= 240 {
			fmt.Printf("  🚨 CRITICAL: %d Standard DNS accounts (limit is 260)\n", count)
			fmt.Printf("  This is likely causing the error: 'Subscription already contains %d storage accounts with Standard Dns endpoints'\n", count)
		} else if count >= 200 {
			fmt.Printf("  ⚠️  WARNING: %d Standard DNS accounts (approaching limit of 260)\n", count)
		}

		// Show oldest accounts that could be candidates for deletion
		if count > 0 {
			fmt.Printf("  Oldest Standard DNS accounts in this location:\n")
			// Sort by creation time (oldest first)
			sortedAccounts := make([]StorageAccountResult, len(accounts))
			copy(sortedAccounts, accounts)
			// Simple sort by creation time (nil times go to end)
			sort.Slice(sortedAccounts, func(i, j int) bool {
				if sortedAccounts[i].CreatedTime == nil {
					return false // Place nil times at the end
				}
				if sortedAccounts[j].CreatedTime == nil {
					return true // Place non-nil times before nil times
				}
				return sortedAccounts[i].CreatedTime.Before(*sortedAccounts[j].CreatedTime)
			})

			// Show top 5 oldest accounts
			for i := 0; i < 5 && i < len(sortedAccounts); i++ {
				sa := sortedAccounts[i].StorageAccount
				createdTime := "Not available"
				if sortedAccounts[i].CreatedTime != nil {
					createdTime = sortedAccounts[i].CreatedTime.Format("2006-01-02")
				}
				fmt.Printf("    - %s (Created: %s)\n", sa.Name, createdTime)
			}
		}
	}

	// Print detailed storage account information
	fmt.Println("\n=== DETAILED STORAGE ACCOUNT INFORMATION ===")
	for _, result := range results {
		if result.Error != nil {
			continue
		}

		sa := result.StorageAccount
		createdTime := "Not available"
		if result.CreatedTime != nil {
			createdTime = result.CreatedTime.Format(time.RFC3339)
		}

		accountType := sa.Properties.AccountType
		if accountType == "" {
			accountType = "Unknown"
		}

		fmt.Printf("\nStorage Account: %s\n", sa.Name)
		fmt.Printf("  Location: %s\n", sa.Location)
		fmt.Printf("  Account Type: %s\n", accountType)
		fmt.Printf("  Provisioning State: %s\n", sa.Properties.ProvisioningState)
		fmt.Printf("  Created: %s\n", createdTime)

		// Show endpoint information
		if sa.Properties.PrimaryEndpoints.Blob != "" {
			fmt.Printf("  Blob Endpoint: %s\n", sa.Properties.PrimaryEndpoints.Blob)
		}
		if sa.Properties.PrimaryEndpoints.Queue != "" {
			fmt.Printf("  Queue Endpoint: %s\n", sa.Properties.PrimaryEndpoints.Queue)
		}
		if sa.Properties.PrimaryEndpoints.Table != "" {
			fmt.Printf("  Table Endpoint: %s\n", sa.Properties.PrimaryEndpoints.Table)
		}
		if sa.Properties.PrimaryEndpoints.File != "" {
			fmt.Printf("  File Endpoint: %s\n", sa.Properties.PrimaryEndpoints.File)
		}
	}

	// Print recommendations
	fmt.Println("\n=== RECOMMENDATIONS ===")
	for location, accountTypes := range locationCounts {
		totalInLocation := 0
		for _, count := range accountTypes {
			totalInLocation += count
		}

		if totalInLocation >= 240 {
			fmt.Printf("Location %s has %d storage accounts:\n", location, totalInLocation)
			fmt.Printf("  - Consider deleting unused storage accounts\n")
			fmt.Printf("  - Review storage accounts created by default services\n")
			fmt.Printf("  - Consider using different regions for new storage accounts\n")
		}
	}

	// Specific recommendations for Standard DNS endpoint issue
	for location, accounts := range standardDNSAccounts {
		count := len(accounts)
		if count >= 200 {
			fmt.Printf("\nFor Standard DNS endpoint issue in %s (%d accounts):\n", location, count)
			fmt.Printf("  - Focus on deleting Standard DNS accounts (Standard_LRS, Standard_GRS, etc.)\n")
			fmt.Printf("  - Check for storage accounts created by Azure services (Cloud Shell, etc.)\n")
			fmt.Printf("  - Consider migrating data to Premium storage accounts if possible\n")
			fmt.Printf("  - Use different regions for new Standard DNS storage accounts\n")
		}
	}
}

// processResourceGroupsConcurrently processes resource groups concurrently for better performance
func (ac *AzureClient) processResourceGroupsConcurrently(resourceGroups []ResourceGroup) {
	var wg sync.WaitGroup
	results := make([]ResourceGroupResult, len(resourceGroups))

	// Ensure MaxConcurrency is at least 1 to prevent hanging
	maxConcurrency := validateConcurrency(ac.Config.MaxConcurrency)

	// Use a semaphore to limit concurrent goroutines
	semaphore := make(chan struct{}, maxConcurrency)

	// Start spinner if not in porcelain mode
	var spinner *Spinner
	if !ac.Config.Porcelain {
		spinner = NewSpinner("Processing resource groups...")
		spinner.Start()
	}

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

	// Stop spinner if it was started
	if spinner != nil {
		spinner.Stop()
	}

	// Print all results
	for _, result := range results {
		ac.printResourceGroupResult(result, false)
	}
}

// processResourceGroupsConcurrentlyWithResources processes resource groups with detailed resource listing
func (ac *AzureClient) processResourceGroupsConcurrentlyWithResources(resourceGroups []ResourceGroup) {
	// Start spinner if not in porcelain mode
	var spinner *Spinner
	if !ac.Config.Porcelain {
		spinner = NewSpinner("Processing resource groups with resources...")
		spinner.Start()
	}

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

	// Stop spinner if it was started
	if spinner != nil {
		spinner.Stop()
	}
}

// printResourceGroupResult prints the result of processing a resource group
func (ac *AzureClient) printResourceGroupResult(result ResourceGroupResult, listResources bool) {
	rg := result.ResourceGroup

	// Check if this is a default resource group
	defaultInfo := checkIfDefaultResourceGroup(rg.Name)

	if ac.Config.Porcelain {
		// Porcelain mode: compact, single-line format for scripts
		createdTime := ""
		if result.Error != nil {
			createdTime = "ERROR"
		} else if result.CreatedTime != nil {
			createdTime = result.CreatedTime.Format(time.RFC3339)
		} else {
			createdTime = "N/A"
		}

		isDefault := "false"
		if defaultInfo.IsDefault {
			isDefault = "true"
		}

		fmt.Printf("%s\t%s\t%s\t%s\t%s\n",
			rg.Name,
			rg.Location,
			rg.Properties.ProvisioningState,
			createdTime,
			isDefault)
	} else {
		// Human-readable format
		fmt.Printf("Resource Group: %s\n", rg.Name)
		fmt.Printf("  Location: %s\n", rg.Location)
		fmt.Printf("  Provisioning State: %s\n", rg.Properties.ProvisioningState)

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

// fetchStorageAccountCreatedTime is no longer needed since we get creation time from the initial API call
// This function is kept for backward compatibility but should not be used
func (ac *AzureClient) fetchStorageAccountCreatedTime(storageAccount StorageAccount) (*time.Time, error) {
	// Simply return the creation time from the storage account if available
	return storageAccount.CreatedTime, nil
}

// extractResourceGroupFromID extracts resource group name from a resource ID
func extractResourceGroupFromID(resourceID string) string {
	parts := strings.Split(resourceID, "/")
	for i, part := range parts {
		if part == "resourceGroups" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
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

// CSV Row structure for resource groups output
type CSVRow struct {
	ResourceGroupName string
	Location          string
	ProvisioningState string
	CreatedTime       string
	IsDefault         string
	CreatedBy         string
	Description       string
	Resources         string
}

// CSV Row structure for storage accounts output
type StorageAccountCSVRow struct {
	StorageAccountName string
	Location           string
	AccountType        string
	ProvisioningState  string
	CreatedTime        string
	ResourceGroup      string
	BlobEndpoint       string
	QueueEndpoint      string
	TableEndpoint      string
	FileEndpoint       string
	Error              string
}

// processResourceGroupsConcurrentlyCSV processes resource groups concurrently and returns CSV data
func (ac *AzureClient) processResourceGroupsConcurrentlyCSV(resourceGroups []ResourceGroup) []CSVRow {
	var wg sync.WaitGroup
	results := make([]ResourceGroupResult, len(resourceGroups))

	// Ensure MaxConcurrency is at least 1 to prevent hanging
	maxConcurrency := validateConcurrency(ac.Config.MaxConcurrency)

	// Use a semaphore to limit concurrent goroutines
	semaphore := make(chan struct{}, maxConcurrency)

	// Start spinner if not in porcelain mode
	var spinner *Spinner
	if !ac.Config.Porcelain {
		spinner = NewSpinner("Processing resource groups for CSV...")
		spinner.Start()
	}

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

	// Stop spinner if it was started
	if spinner != nil {
		spinner.Stop()
	}

	// Convert results to CSV format
	csvData := make([]CSVRow, 0, len(results))
	for _, result := range results {
		csvRow := ac.convertToCSVRow(result, false, nil)
		csvData = append(csvData, csvRow)
		// Also print to console
		ac.printResourceGroupResult(result, false)
	}

	return csvData
}

// processResourceGroupsConcurrentlyWithResourcesCSV processes resource groups with resources and returns CSV data
func (ac *AzureClient) processResourceGroupsConcurrentlyWithResourcesCSV(resourceGroups []ResourceGroup) []CSVRow {
	csvData := make([]CSVRow, 0, len(resourceGroups))

	// Start spinner if not in porcelain mode
	var spinner *Spinner
	if !ac.Config.Porcelain {
		spinner = NewSpinner("Processing resource groups with resources for CSV...")
		spinner.Start()
	}

	for _, rg := range resourceGroups {
		// Fetch resources for this resource group
		resources, err := ac.fetchResourcesInGroup(rg.Name)
		if err != nil {
			// Create a result with error
			result := ResourceGroupResult{
				ResourceGroup: rg,
				CreatedTime:   nil,
				Error:         err,
			}
			csvRow := ac.convertToCSVRow(result, true, nil)
			csvData = append(csvData, csvRow)
			ac.printResourceGroupResult(result, true)
			continue
		}

		// Create result with resources
		result := ResourceGroupResult{
			ResourceGroup: rg,
			CreatedTime:   nil, // Will be calculated from resources
			Error:         nil,
		}
		csvRow := ac.convertToCSVRow(result, true, resources)
		csvData = append(csvData, csvRow)
		ac.printResourceGroupResultWithResources(result, resources)
	}

	// Stop spinner if it was started
	if spinner != nil {
		spinner.Stop()
	}

	return csvData
}

// fetchResourcesInGroup fetches resources in a resource group and returns them
func (ac *AzureClient) fetchResourcesInGroup(resourceGroupName string) ([]Resource, error) {
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

	return resourcesResponse.Value, nil
}

// convertToCSVRow converts a ResourceGroupResult to a CSVRow
func (ac *AzureClient) convertToCSVRow(result ResourceGroupResult, listResources bool, resources []Resource) CSVRow {
	rg := result.ResourceGroup

	// Check if this is a default resource group
	defaultInfo := checkIfDefaultResourceGroup(rg.Name)

	// Format created time
	createdTimeStr := ""
	if result.Error != nil {
		createdTimeStr = "Error: " + result.Error.Error()
	} else if result.CreatedTime != nil {
		createdTimeStr = result.CreatedTime.Format(time.RFC3339)
	} else {
		createdTimeStr = "Not available"
	}

	// Format resources as a single field if listResources is true
	resourcesStr := ""
	if listResources && resources != nil {
		resourcesList := make([]string, 0, len(resources))
		for _, resource := range resources {
			resourceInfo := fmt.Sprintf("%s (%s)", resource.Name, resource.Type)
			if resource.CreatedTime != nil {
				resourceInfo += " - Created: " + resource.CreatedTime.Format(time.RFC3339)
			} else {
				resourceInfo += " - Created: Not available"
			}
			resourcesList = append(resourcesList, resourceInfo)
		}
		resourcesStr = strings.Join(resourcesList, "; ")
	}

	return CSVRow{
		ResourceGroupName: rg.Name,
		Location:          rg.Location,
		ProvisioningState: rg.Properties.ProvisioningState,
		CreatedTime:       createdTimeStr,
		IsDefault:         fmt.Sprintf("%v", defaultInfo.IsDefault),
		CreatedBy:         defaultInfo.CreatedBy,
		Description:       defaultInfo.Description,
		Resources:         resourcesStr,
	}
}

// printResourceGroupResultWithResources prints a resource group result with resources
func (ac *AzureClient) printResourceGroupResultWithResources(result ResourceGroupResult, resources []Resource) {
	rg := result.ResourceGroup

	// Check if this is a default resource group
	defaultInfo := checkIfDefaultResourceGroup(rg.Name)

	if ac.Config.Porcelain {
		// For porcelain mode, we need to get creation time from resources
		createdTime := ""
		if len(resources) > 0 {
			// Find the earliest created time among all resources
			var earliestTime *time.Time
			for _, resource := range resources {
				if resource.CreatedTime != nil {
					if earliestTime == nil || resource.CreatedTime.Before(*earliestTime) {
						earliestTime = resource.CreatedTime
					}
				}
			}
			if earliestTime != nil {
				createdTime = earliestTime.Format(time.RFC3339)
			} else {
				createdTime = "N/A"
			}
		} else {
			createdTime = "N/A"
		}

		isDefault := "false"
		if defaultInfo.IsDefault {
			isDefault = "true"
		}

		fmt.Printf("%s\t%s\t%s\t%s\t%s\n",
			rg.Name,
			rg.Location,
			rg.Properties.ProvisioningState,
			createdTime,
			isDefault)
	} else {
		// Human-readable format
		fmt.Printf("Resource Group: %s\n", rg.Name)
		fmt.Printf("  Location: %s\n", rg.Location)
		fmt.Printf("  Provisioning State: %s\n", rg.Properties.ProvisioningState)

		if defaultInfo.IsDefault {
			fmt.Printf("  🔍 DEFAULT RESOURCE GROUP DETECTED\n")
			fmt.Printf("  📋 Created By: %s\n", defaultInfo.CreatedBy)
			fmt.Printf("  📝 Description: %s\n", defaultInfo.Description)
		}

		// Print resources
		if len(resources) == 0 {
			fmt.Printf("  No resources found in this resource group\n")
		} else {
			fmt.Printf("  Resources (%d):\n", len(resources))
			for _, resource := range resources {
				fmt.Printf("    - %s (%s)\n", resource.Name, resource.Type)
				if resource.CreatedTime != nil {
					fmt.Printf("      Created: %s\n", resource.CreatedTime.Format(time.RFC3339))
				} else {
					fmt.Printf("      Created: Not available\n")
				}
			}
		}

		fmt.Println()
	}
}

// writeCSVFile writes CSV data to the specified file
func (ac *AzureClient) writeCSVFile(csvData []CSVRow) error {
	file, err := os.Create(ac.Config.OutputCSV)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Warning: failed to close CSV file: %v", err)
		}
	}()

	writer := csv.NewWriter(file)
	defer func() {
		writer.Flush()
		if err := writer.Error(); err != nil {
			log.Printf("Warning: failed to flush CSV writer: %v", err)
		}
	}()

	// Write header
	header := []string{
		"ResourceGroupName",
		"Location",
		"ProvisioningState",
		"CreatedTime",
		"IsDefault",
		"CreatedBy",
		"Description",
		"Resources",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write data rows
	for _, row := range csvData {
		record := []string{
			row.ResourceGroupName,
			row.Location,
			row.ProvisioningState,
			row.CreatedTime,
			row.IsDefault,
			row.CreatedBy,
			row.Description,
			row.Resources,
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	return nil
}

// writeStorageAccountCSVFile writes storage account CSV data to the specified file
func (ac *AzureClient) writeStorageAccountCSVFile(csvData []StorageAccountCSVRow) error {
	file, err := os.Create(ac.Config.OutputCSV)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Warning: failed to close CSV file: %v", err)
		}
	}()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{
		"StorageAccountName",
		"Location",
		"AccountType",
		"ProvisioningState",
		"CreatedTime",
		"ResourceGroup",
		"BlobEndpoint",
		"QueueEndpoint",
		"TableEndpoint",
		"FileEndpoint",
		"Error",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write data rows
	for _, row := range csvData {
		record := []string{
			row.StorageAccountName,
			row.Location,
			row.AccountType,
			row.ProvisioningState,
			row.CreatedTime,
			row.ResourceGroup,
			row.BlobEndpoint,
			row.QueueEndpoint,
			row.TableEndpoint,
			row.FileEndpoint,
			row.Error,
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
