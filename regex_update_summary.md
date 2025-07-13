# Default Resource Group Regex Updates

## Problem
Several default resource groups were not being detected as default due to insufficient regex patterns. The examples provided included:

### Missing Patterns
- `Default-ActivityLogAlerts`
- `Default-EventHub-EastUS`
- `Default-ServiceBus-CentralUS`
- `Default-SQL-JapanWest`
- `Default-Storage-EastUS`
- `Default-Storage-NorthCentralUS`
- `Default-Storage-NorthEurope`
- `Default-Storage-SouthCentralUS`
- `Default-Storage-WestEurope`
- `Default-Storage-WestUS`
- `Default-Web-JapanWest`
- `Default-Web-SouthCentralUS`
- `cloud-shell-storage-centralindia`
- `cloud-shell-storage-eastus`
- `cloud-shell-storage-northeurope`
- `cloud-shell-storage-southcentralus`
- `cloud-shell-storage-westeurope`

## Solution
Added two new regex patterns to handle these missing patterns:

### 1. Default Service Pattern
```go
defaultServicePattern = regexp.MustCompile(`^default-[a-z0-9]+(-[a-z0-9]+)*$`)
```

**Matches:**
- `Default-ActivityLogAlerts`
- `Default-EventHub-EastUS`
- `Default-ServiceBus-CentralUS`
- `Default-SQL-JapanWest`
- `Default-Storage-*` (all variants)
- `Default-Web-*` (all variants)

**Detection Info:**
- Created By: "Azure Services"
- Description: "Default resource group created by Azure services for regional deployments"

### 2. Cloud Shell Storage Pattern
```go
cloudShellStoragePattern = regexp.MustCompile(`^cloud-shell-storage-[a-z0-9]+$`)
```

**Matches:**
- `cloud-shell-storage-centralindia`
- `cloud-shell-storage-eastus`
- `cloud-shell-storage-northeurope`
- `cloud-shell-storage-southcentralus`
- `cloud-shell-storage-westeurope`

**Detection Info:**
- Created By: "Azure Cloud Shell"
- Description: "Default storage resource group created by Azure Cloud Shell for persistent storage"

## Changes Made

### 1. Updated Pattern Definitions (`main.go` lines 20-30)
```go
// Pre-compiled regex patterns for better performance
var (
    defaultResourceGroupPattern = regexp.MustCompile(`^defaultresourcegroup-`)
    defaultServicePattern       = regexp.MustCompile(`^default-[a-z0-9]+(-[a-z0-9]+)*$`)      // NEW
    cloudShellStoragePattern    = regexp.MustCompile(`^cloud-shell-storage-[a-z0-9]+$`)        // NEW
    dynamicsPattern             = regexp.MustCompile(`^dynamicsdeployments$`)
    // ... other existing patterns
)
```

### 2. Updated Detection Logic (`main.go` checkIfDefaultResourceGroup function)
Added detection logic for the new patterns:

```go
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
```

### 3. Added Test Cases (`main_test.go`)
Added comprehensive test cases for the new patterns:

- `Default-Storage-EastUS pattern`
- `Default-EventHub-EastUS pattern`
- `Default-ActivityLogAlerts pattern`
- `Default-SQL-JapanWest pattern`
- `cloud-shell-storage-eastus pattern`
- `cloud-shell-storage-centralindia pattern`
- `cloud-shell-storage-westeurope pattern`

## Validation
All provided examples now pass validation:
- ✅ All 17 example resource groups are now properly detected as default
- ✅ All existing tests continue to pass
- ✅ New test cases added to prevent regression

## Pattern Coverage
The updated system now detects these types of default resource groups:

1. **DefaultResourceGroup-XXX** (existing)
2. **Default-ServiceName-Region** (new)
3. **cloud-shell-storage-region** (new)
4. **DynamicsDeployments** (existing)
5. **MC_*_*_*** (AKS, existing)
6. **AzureBackupRG*** (existing)
7. **NetworkWatcherRG** (existing)
8. **databricks-rg*** (existing)
9. **microsoft-network** (existing)
10. **LogAnalyticsDefaultResources** (existing)