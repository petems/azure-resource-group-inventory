
# azure-resource-group-inventory

`azrginventory` is a command-line tool to get a full inventory of all your Azure resource groups within a subscription, including when each group was created (based on the earliest resource in the group).


## Features

- Fetches all resource groups from a specified Azure subscription
- Determines the creation time for each resource group, based on the earliest resource in the group
- **🆕 Detects default Azure resource groups** and shows context about what created them
- Supports both command-line flags and environment variables for configuration
- Clean, formatted output with resource group details


## Default Resource Group Detection

`azrginventory` automatically identifies default resource groups created by Azure services and provides context about what created them:

| Pattern | Created By | Description |
|---------|------------|-------------|
| `DefaultResourceGroup-XXX` | Azure CLI / Cloud Shell / Visual Studio | Common default resource group for the region |
| `DynamicsDeployments` | Microsoft Dynamics ERP | Automatically created for non-production instances |
| `MC_*_*_*` | Azure Kubernetes Service (AKS) | Contains AKS cluster infrastructure resources |
| `AzureBackupRG*` | Azure Backup | Created for backup operations |
| `NetworkWatcherRG` | Azure Network Watcher | Created for network monitoring |
| `databricks-rg*` | Azure Databricks | Created for managed workspace resources |
| `microsoft-network` | Microsoft Networking Services | Used by Microsoft's networking services |
| `LogAnalyticsDefaultResources` | Azure Log Analytics | Created for default workspace resources |


## Prerequisites

- Go 1.21 or higher
- Azure subscription
- Azure access token for authentication


## Installation

1. Clone this repository
2. Install dependencies:
   ```bash
   go mod tidy
   ```
3. Build the application:
   ```bash
   go build -o azrginventory
   ```

## Context: Why was this tool created?

This tool was created to address a real-world problem: managing a large Azure sandbox subscription with 900+ resource groups, unclear ownership, and uncertainty about what is still in use. Azure imposes a hard cap of `980` [on the number of resource groups per subscription](https://learn.microsoft.com/en-us/azure/azure-resource-manager/management/azure-subscription-service-limits#azure-subscription-limits), making it critical to inventory, audit, and clean up unused or unknown groups. 

`azrginventory` helps you quickly understand what exists, when it was created, and which groups may be default or system-generated, and can output to csv format for planning and assesment within teams.

## Getting Azure Access Token

To use this tool, you need an Azure access token. Here are a few ways to obtain one:

### Option 1: Using Azure CLI
```bash
# Login to Azure
az login

# Get access token
az account get-access-token --resource https://management.azure.com/
```

### Option 2: Using Azure PowerShell
```powershell
# Login to Azure
Connect-AzAccount

# Get access token
(Get-AzAccessToken -ResourceUrl "https://management.azure.com/").Token
```

### Option 3: Using REST API with Service Principal
You can also use a service principal for authentication. This requires:
- Application (client) ID
- Directory (tenant) ID
- Client secret

Then make a POST request to get the token:
```bash
curl -X POST https://login.microsoftonline.com/{tenant-id}/oauth2/v2.0/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "client_id={client-id}&scope=https://management.azure.com/.default&client_secret={client-secret}&grant_type=client_credentials"
```

## Usage

### Using Command Line Flags
```bash
./azrginventory --subscription-id "your-subscription-id" --access-token "your-access-token"
```

### Using Environment Variables
```bash
export AZURE_SUBSCRIPTION_ID="your-subscription-id"
export AZURE_ACCESS_TOKEN="your-access-token"
./azrginventory
```

### Example Output
```
Fetching resource groups...
Found 5 resource groups:

Resource Group: DefaultResourceGroup-EUS
  Location: eastus
  Provisioning State: Succeeded
  🔍 DEFAULT RESOURCE GROUP DETECTED
  📋 Created By: Azure CLI / Cloud Shell / Visual Studio
  📝 Description: Common default resource group created for the region, used by Azure CLI, Cloud Shell, and Visual Studio for resource deployment
  Created Time: 2023-10-15T14:30:22Z

Resource Group: MC_myapp_myakscluster_eastus
  Location: eastus
  Provisioning State: Succeeded
  🔍 DEFAULT RESOURCE GROUP DETECTED
  📋 Created By: Azure Kubernetes Service (AKS)
  📝 Description: Created when deploying an AKS cluster, contains infrastructure resources for the cluster
  Created Time: 2023-11-01T09:15:45Z

Resource Group: my-app-rg
  Location: westus2
  Provisioning State: Succeeded
  Created Time: 2023-10-20T16:45:12Z

Resource Group: NetworkWatcherRG
  Location: eastus
  Provisioning State: Succeeded
  🔍 DEFAULT RESOURCE GROUP DETECTED
  📋 Created By: Azure Network Watcher
  📝 Description: Created by Azure Network Watcher service for network monitoring
  Created Time: Not available

Resource Group: databricks-rg-myworkspace-abc123
  Location: westus
  Provisioning State: Succeeded
  🔍 DEFAULT RESOURCE GROUP DETECTED
  📋 Created By: Azure Databricks
  📝 Description: Created by Azure Databricks service for managed workspace resources
  Created Time: 2023-11-05T11:20:33Z
```

## Configuration

The tool accepts configuration via:

1. **Command line flags:**
   - `--subscription-id`: Azure subscription ID
   - `--access-token`: Azure access token

2. **Environment variables:**
   - `AZURE_SUBSCRIPTION_ID`: Azure subscription ID
   - `AZURE_ACCESS_TOKEN`: Azure access token

Command line flags take precedence over environment variables.

## How It Works

1. **Fetch Resource Groups**: Uses the Azure Management API to get all resource groups:
   ```
   GET https://management.azure.com/subscriptions/{subscription-id}/resourcegroups?api-version=2021-04-01
   ```

2. **Get Creation Times**: For each resource group, fetches its resources with creation time:
   ```
   GET https://management.azure.com/subscriptions/{subscription-id}/resourceGroups/{resource-group-name}/resources?$expand=createdTime&api-version=2019-10-01
   ```

3. **Display Results**: Shows resource group details including the earliest creation time found among its resources.

## Error Handling

The tool includes comprehensive error handling for:
- Missing configuration (subscription ID or access token)
- Network connectivity issues
- Azure API authentication failures
- Invalid API responses
- JSON parsing errors

## Contributing

Feel free to submit issues and enhancement requests!

## License

This project is licensed under the MIT License - see the LICENSE file for details.
