
# azure-resource-group-inventory

`azrginventory` is a command-line tool to get a full inventory of all your Azure resource groups within a subscription, including when each group was created (based on the earliest resource in the group).

## Features

- Fetches all resource groups from a specified Azure subscription
- Determines the creation time for each resource group, based on the earliest resource in the group
- **🆕 Detects default Azure resource groups** and shows context about what created them
- **🆕 Lists storage accounts with creation times** and identifies location-based limits
- Clean, formatted output with resource group details

## Commands

### Resource Groups (Default)
Lists all resource groups with their creation times:
```bash
azrginventory --subscription-id <id> --access-token <token>
```

### Storage Accounts
Lists all storage accounts with creation times and analyzes location-based limits:
```bash
azrginventory storage-accounts --subscription-id <id> --access-token <token>
```

The storage accounts command specifically helps address issues like:
> "Subscription already contains 260 storage accounts with Standard Dns endpoints in location eastus and the maximum allowed is 260."

## Default Resource Group Detection

`azrginventory` automatically identifies default resource groups created by Azure services and provides context about what created them:

| Pattern | Created By | Description |
|---------|------------|-------------|
| `DefaultResourceGroup-XXX` | Azure CLI / Cloud Shell / Visual Studio | Common default resource group for the region |
| `Default-XXX` | Azure Services | Default resource group created by Azure services for regional deployments |
| `cloud-shell-storage-XXX` | Azure Cloud Shell | Default storage resource group created by Azure Cloud Shell for persistent storage |
| `MC_XXX_XXX_XXX` | Azure Kubernetes Service (AKS) | Managed cluster resource group for AKS |
| `NetworkWatcherRG` | Azure Network Watcher | Default resource group for Network Watcher |
| `databricks-rg-XXX` | Azure Databricks | Default resource group for Databricks workspaces |
| `microsoft-network` | Azure Networking | Default resource group for networking services |
| `loganalyticsdefaultresources` | Azure Monitor | Default resource group for Log Analytics |

## Storage Account Analysis

The storage accounts command provides:

- **Location-based summary**: Shows storage account counts by location and account type
- **Standard DNS endpoint analysis**: Specifically tracks Standard DNS accounts that cause limit issues
- **Creation time tracking**: Shows when each storage account was created
- **Limit warnings**: Alerts when approaching Azure's 260 storage account limit per region
- **Deletion recommendations**: Identifies oldest accounts that could be candidates for deletion
- **Endpoint information**: Shows blob, queue, table, and file endpoints for each account

## Installation

### Prerequisites

- Go 1.19 or later
- Azure subscription with appropriate permissions
- Azure access token

### Build from source

```bash
git clone <repository-url>
cd azure-resource-group-inventory
go build -o azrginventory
```

## Usage

### Authentication

You need an Azure access token. You can get one using the Azure CLI:

```bash
az account get-access-token --resource https://management.azure.com
```

**Quick Setup Helper:**
```bash
# Login and set up environment variables in one go
az login && export AZURE_SUBSCRIPTION_ID=$(az account show --query id -o tsv) && export AZURE_ACCESS_TOKEN=$(az account get-access-token --query accessToken -o tsv)
```

### Basic Usage

```bash
# List resource groups
./azrginventory --subscription-id <subscription-id> --access-token <access-token>

# List storage accounts
./azrginventory storage-accounts --subscription-id <subscription-id> --access-token <access-token>
```

### Environment Variables

You can also set the credentials via environment variables:

```bash
export AZURE_SUBSCRIPTION_ID="your-subscription-id"
export AZURE_ACCESS_TOKEN="your-access-token"
./azrginventory
```

### Advanced Options

```bash
# List resources within each resource group
./azrginventory --list-resources --subscription-id <id> --access-token <token>

# Output to CSV file
./azrginventory --output-csv results.csv --subscription-id <id> --access-token <token>

# Machine-readable output (tab-separated)
./azrginventory --porcelain --subscription-id <id> --access-token <token>

# Control concurrency (default: 10)
./azrginventory --max-concurrency 20 --subscription-id <id> --access-token <token>
```

## Example Output

### Resource Groups
```
Fetching resource groups...
Found 5 resource groups:

Resource Group: DefaultResourceGroup-EUS
  Location: eastus
  Provisioning State: Succeeded
  Created: 2023-01-15T10:30:00Z
  🔍 DEFAULT RESOURCE GROUP DETECTED
  📋 Created By: Azure CLI / Cloud Shell / Visual Studio
  📝 Description: Common default resource group created for the region, used by Azure CLI, Cloud Shell, and Visual Studio for resource deployment

Resource Group: MC_myapp_myakscluster_eastus
  Location: eastus
  Provisioning State: Succeeded
  Created: 2023-02-20T14:45:00Z
  🔍 DEFAULT RESOURCE GROUP DETECTED
  📋 Created By: Azure Kubernetes Service (AKS)
  📝 Description: Managed cluster resource group for AKS

Resource Group: my-app-rg
  Location: eastus
  Provisioning State: Succeeded
  Created: 2023-03-10T09:15:00Z
  Resources (2):
    - my-app-storage (Microsoft.Storage/storageAccounts)
      Created: 2023-03-10T09:15:00Z
    - my-app-web (Microsoft.Web/sites)
      Created: 2023-03-10T09:20:00Z
```

### Storage Accounts
```
Fetching storage accounts...
Found 15 storage accounts:

=== STORAGE ACCOUNT SUMMARY BY LOCATION ===

Location: eastus
  Standard_LRS: 245 accounts
  Premium_LRS: 5 accounts
  Total: 250 accounts
  ⚠️  WARNING: Approaching limit of 250 storage accounts per region!

=== STANDARD DNS ENDPOINT ANALYSIS ===

Location: eastus - Standard DNS accounts: 245
  🚨 CRITICAL: 245 Standard DNS accounts (limit is 260)
  This is likely causing the error: 'Subscription already contains 245 storage accounts with Standard Dns endpoints'
  Oldest Standard DNS accounts in this location:
    - cloudshellstorage123 (Created: 2020-01-15)
    - defaultstorage456 (Created: 2020-03-20)
    - teststorage789 (Created: 2020-05-10)

=== RECOMMENDATIONS ===

For Standard DNS endpoint issue in eastus (245 accounts):
  - Focus on deleting Standard DNS accounts (Standard_LRS, Standard_GRS, etc.)
  - Check for storage accounts created by Azure services (Cloud Shell, etc.)
  - Consider migrating data to Premium storage accounts if possible
  - Use different regions for new Standard DNS storage accounts
```

## Why This Tool?

This tool was created to address a real-world problem: managing a large Azure sandbox subscription with 900+ resource groups, unclear ownership, and uncertainty about what is still in use. Azure imposes a hard cap of `980` [on the number of resource groups per subscription](https://learn.microsoft.com/en-us/azure/azure-resource-manager/management/azure-subscription-service-limits#resource-groups), and `260` storage accounts per region, making it crucial to identify and clean up unused resources.

The storage accounts feature specifically addresses the common issue where subscriptions hit the 260 storage account limit per region, particularly with Standard DNS endpoints that are commonly created by Azure services like Cloud Shell.

## Performance

The tool is optimized for performance with large subscriptions:

- **Concurrent API calls**: Processes multiple resource groups/storage accounts simultaneously
- **Configurable concurrency**: Control the number of concurrent requests (default: 10)
- **Efficient resource fetching**: Minimizes API calls by batching requests
- **Progress feedback**: Shows spinner during long operations

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
