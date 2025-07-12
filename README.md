# Azure Resource Group CLI

A command-line tool to fetch Azure resource groups and their creation times using the Azure Management API.

## Features

- Fetches all resource groups from a specified Azure subscription
- Retrieves creation times for each resource group
- Supports both command-line flags and environment variables for configuration
- Clean, formatted output with resource group details

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
   go build -o azure-rg-cli
   ```

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
./azure-rg-cli --subscription-id "your-subscription-id" --access-token "your-access-token"
```

### Using Environment Variables
```bash
export AZURE_SUBSCRIPTION_ID="your-subscription-id"
export AZURE_ACCESS_TOKEN="your-access-token"
./azure-rg-cli
```

### Example Output
```
Fetching resource groups...
Found 3 resource groups:

Resource Group: my-app-rg
  Location: eastus
  Provisioning State: Succeeded
  Created Time: 2023-10-15T14:30:22Z

Resource Group: test-rg
  Location: westus2
  Provisioning State: Succeeded
  Created Time: 2023-11-01T09:15:45Z

Resource Group: backup-rg
  Location: centralus
  Provisioning State: Succeeded
  Created Time: Not available
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
