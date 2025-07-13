# CSV Output Functionality Demo

## Overview
This document demonstrates the CSV output functionality that has been added to the Azure Resource Groups CLI tool.

## New Features Added

### 1. CSV Output Flag
- Added `--output-csv` flag to specify the output CSV file path
- When this flag is used, the tool will write results to a CSV file in addition to console output

### 2. CSV Structure
The CSV file contains the following columns:
- **ResourceGroupName**: Name of the resource group
- **Location**: Azure region where the resource group is located
- **ProvisioningState**: Current provisioning state (e.g., "Succeeded")
- **CreatedTime**: Creation timestamp or error message
- **IsDefault**: Boolean indicating if this is a default resource group
- **CreatedBy**: Service that created the resource group (for default RGs)
- **Description**: Description of the resource group's purpose (for default RGs)
- **Resources**: List of all resources in the group (when --list-resources is used)

### 3. Resource Listing in CSV
When the `--list-resources` flag is combined with `--output-csv`, all resources within each resource group are consolidated into a single CSV field, formatted as:
```
resource-name (resource-type) - Created: timestamp; resource-name2 (resource-type2) - Created: timestamp
```

## Usage Examples

### Basic CSV Output
```bash
./azure-rg-cli --subscription-id "your-sub-id" --access-token "your-token" --output-csv "output.csv"
```

### CSV Output with Resource Listing
```bash
./azure-rg-cli --subscription-id "your-sub-id" --access-token "your-token" --list-resources --output-csv "output_with_resources.csv"
```

## Sample CSV Output

### Without Resources
```csv
ResourceGroupName,Location,ProvisioningState,CreatedTime,IsDefault,CreatedBy,Description,Resources
test-rg-1,eastus,Succeeded,2023-01-01T12:00:00Z,false,,,
DefaultResourceGroup-EUS,eastus,Succeeded,2023-01-01T12:00:00Z,true,Azure CLI / Cloud Shell / Visual Studio,Common default resource group created for the region,
```

### With Resources
```csv
ResourceGroupName,Location,ProvisioningState,CreatedTime,IsDefault,CreatedBy,Description,Resources
test-rg-1,eastus,Succeeded,Not available,false,,,test-storage (Microsoft.Storage/storageAccounts) - Created: 2023-01-01T12:00:00Z; test-app (Microsoft.Web/sites) - Created: 2023-01-02T12:00:00Z
```

## Key Features

1. **Concurrent Processing**: CSV generation maintains the same concurrent processing performance as console output
2. **Default Resource Group Detection**: Default resource groups are properly identified and marked in the CSV
3. **Error Handling**: Errors are captured and included in the CSV output
4. **Resource Consolidation**: When using `--list-resources`, all resources are consolidated into a single CSV field as requested
5. **Backwards Compatibility**: Existing functionality remains unchanged; CSV output is purely additive

## Testing
The implementation includes comprehensive tests covering:
- CSV output without resources
- CSV output with resources (consolidated in single field)
- Default resource group detection in CSV
- Empty resource groups handling
- Error handling in CSV output
- Configuration validation
- File creation and content verification

## Implementation Details
- Added `encoding/csv` package for proper CSV formatting
- New `CSVRow` struct to represent CSV data structure
- New functions for CSV-specific processing:
  - `processResourceGroupsConcurrentlyCSV()`
  - `processResourceGroupsConcurrentlyWithResourcesCSV()`
  - `fetchResourcesInGroup()`
  - `convertToCSVRow()`
  - `writeCSVFile()`
- Updated configuration structure to include `OutputCSV` field
- Added proper flag binding and validation

The CSV functionality is fully tested and ready for production use.