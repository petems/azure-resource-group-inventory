# Storage Accounts Feature Demonstration

This document demonstrates the new storage accounts feature that helps address Azure storage account limit issues.

## Problem Statement

The user encountered this error:
```
Subscription 8c56d827-5f07-45ce-8f2b-6c5001db5c6f already contains 260 storage accounts with Standard Dns endpoints in location eastus and the maximum allowed is 260.
```

This happens when a subscription hits Azure's limit of 260 storage accounts per region, particularly with Standard DNS endpoints.

## Solution: Storage Accounts Command

The new `storage-accounts` command helps identify and manage storage account limits:

### Basic Usage

```bash
# List all storage accounts with creation times and limit analysis
./azrginventory storage-accounts --subscription-id <subscription-id> --access-token <access-token>
```

### Example Output

```
Fetching storage accounts...
Found 260 storage accounts:

=== STORAGE ACCOUNT SUMMARY BY LOCATION ===

Location: eastus
  Standard_LRS: 245 accounts
  Standard_GRS: 10 accounts
  Standard_RAGRS: 5 accounts
  Premium_LRS: 0 accounts
  Total: 260 accounts
  🚨 ERROR: At limit of 250 storage accounts per region!

=== STANDARD DNS ENDPOINT ANALYSIS ===

Location: eastus - Standard DNS accounts: 260
  🚨 CRITICAL: 260 Standard DNS accounts (limit is 260)
  This is likely causing the error: 'Subscription already contains 260 storage accounts with Standard Dns endpoints'
  Oldest Standard DNS accounts in this location:
    - cloudshellstorage123 (Created: 2020-01-15)
    - defaultstorage456 (Created: 2020-03-20)
    - teststorage789 (Created: 2020-05-10)
    - tempstorage101 (Created: 2020-07-15)
    - devstorage202 (Created: 2020-09-30)

=== DETAILED STORAGE ACCOUNT INFORMATION ===

Storage Account: cloudshellstorage123
  Location: eastus
  Account Type: Standard_LRS
  Provisioning State: Succeeded
  Created: 2020-01-15T10:30:00Z
  Blob Endpoint: https://cloudshellstorage123.blob.core.windows.net/
  Queue Endpoint: https://cloudshellstorage123.queue.core.windows.net/
  Table Endpoint: https://cloudshellstorage123.table.core.windows.net/
  File Endpoint: https://cloudshellstorage123.file.core.windows.net/

=== RECOMMENDATIONS ===

For Standard DNS endpoint issue in eastus (260 accounts):
  - Focus on deleting Standard DNS accounts (Standard_LRS, Standard_GRS, etc.)
  - Check for storage accounts created by Azure services (Cloud Shell, etc.)
  - Consider migrating data to Premium storage accounts if possible
  - Use different regions for new Standard DNS storage accounts
```

## Key Features

### 1. Location-Based Analysis
- Groups storage accounts by location
- Shows counts by account type (Standard_LRS, Premium_LRS, etc.)
- Identifies when approaching or at limits

### 2. Standard DNS Endpoint Tracking
- Specifically tracks Standard DNS accounts that cause limit issues
- Shows oldest accounts that could be candidates for deletion
- Provides targeted recommendations

### 3. Creation Time Analysis
- Shows when each storage account was created
- Helps identify old, potentially unused accounts
- Sorts accounts by age for deletion prioritization

### 4. Limit Warnings
- Warns when approaching the 260 account limit
- Shows critical alerts when at the limit
- Provides specific error context

## Action Plan

Based on the output, here's what you can do:

1. **Identify Old Accounts**: Look at the "Oldest Standard DNS accounts" section
2. **Check for Azure Service Accounts**: Look for accounts created by Cloud Shell, etc.
3. **Delete Unused Accounts**: Start with the oldest accounts that are no longer needed
4. **Consider Migration**: Move data to Premium accounts if possible
5. **Use Different Regions**: Create new storage accounts in different regions

## Commands to Help

```bash
# Get current storage account status
./azrginventory storage-accounts --subscription-id <id> --access-token <token>

# After cleanup, verify the count has decreased
./azrginventory storage-accounts --subscription-id <id> --access-token <token>

# Use porcelain mode for script automation
./azrginventory storage-accounts --porcelain --subscription-id <id> --access-token <token>
```

## Integration with Existing Workflow

The storage accounts command integrates seamlessly with the existing resource group inventory:

```bash
# First, check resource groups for context
./azrginventory --subscription-id <id> --access-token <token>

# Then, analyze storage accounts specifically
./azrginventory storage-accounts --subscription-id <id> --access-token <token>

# Export to CSV for team analysis
./azrginventory storage-accounts --output-csv storage_analysis.csv --subscription-id <id> --access-token <token>
```

This feature specifically addresses the storage account limit issue by providing clear visibility into which accounts are causing the problem and which ones could be safely deleted.