# Testing Documentation

## Overview

This project now includes comprehensive unit tests and GitHub Actions CI/CD pipeline for the Azure Resource Groups CLI tool.

## Test Coverage

Current test coverage: **~67%**

### Tested Functions

- `makeAzureRequest` - 84.6% coverage
- `FetchResourceGroups` - 84.0% coverage  
- `fetchResourceGroupCreatedTime` - 88.2% coverage
- Legacy wrapper functions - 100% coverage

### Test Cases

1. **HTTP Request Testing**
   - Valid Azure API requests with proper headers
   - Error handling for failed requests
   - JSON parsing and response validation

2. **Resource Group Fetching**
   - Fetching multiple resource groups
   - Handling empty responses
   - Output formatting validation

3. **Created Time Fetching**
   - Finding earliest creation time among resources
   - Handling resource groups with no resources
   - Time parsing and comparison

4. **Error Handling**
   - Invalid JSON responses
   - Network failures
   - HTTP error status codes

5. **Configuration Testing**
   - Environment variable handling
   - Configuration validation

## Running Tests

```bash
# Run all tests
go test -v ./...

# Run tests with coverage
go test -v -coverprofile=coverage.out ./...

# Generate coverage report
go tool cover -html=coverage.out -o coverage.html

# View function-level coverage
go tool cover -func=coverage.out
```

## GitHub Actions CI/CD

The project includes a comprehensive GitHub Actions workflow (`.github/workflows/ci.yml`) that:

### Test Job
- Runs on Ubuntu latest
- Uses Go 1.21
- Caches Go modules for faster builds
- Runs tests with verbose output
- Generates coverage reports
- Uploads coverage to Codecov
- Builds the application
- Tests the binary

### Lint Job
- Runs golangci-lint for code quality
- Uses timeout of 3 minutes
- Runs in parallel with tests

### Triggers
- Pushes to `master` branch
- Pull requests to `master` branch

## Test Architecture

The tests use dependency injection with an `HTTPClient` interface to mock HTTP requests:

```go
type HTTPClient interface {
    Do(req *http.Request) (*http.Response, error)
}

type MockHTTPClient struct {
    DoFunc func(req *http.Request) (*http.Response, error)
}
```

This allows for comprehensive testing without making actual HTTP calls to Azure APIs.

## Future Improvements

1. **Integration Tests**: Add tests that verify the actual Azure API integration
2. **Performance Tests**: Add benchmarks for large resource group responses
3. **CLI Testing**: Add tests for command-line flag parsing and validation
4. **Error Scenarios**: Add more edge case testing for network failures
5. **Configuration Tests**: Add tests for the `initConfig` function (requires refactoring to avoid `log.Fatal`)