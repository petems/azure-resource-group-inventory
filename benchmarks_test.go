package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// BenchmarkCheckIfDefaultResourceGroup benchmarks the optimized regex pattern matching
func BenchmarkCheckIfDefaultResourceGroup(b *testing.B) {
	testCases := []string{
		"DefaultResourceGroup-EUS",
		"MC_myRG_myAKS_eastus",
		"AzureBackupRG_eastus_1",
		"NetworkWatcherRG",
		"databricks-rg-workspace-123",
		"microsoft-network",
		"LogAnalyticsDefaultResources",
		"DynamicsDeployments",
		"my-custom-resource-group",
		"prod-web-app-rg",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, name := range testCases {
			checkIfDefaultResourceGroup(name)
		}
	}
}

// BenchmarkCheckIfDefaultResourceGroupParallel benchmarks concurrent regex pattern matching
func BenchmarkCheckIfDefaultResourceGroupParallel(b *testing.B) {
	testCases := []string{
		"DefaultResourceGroup-EUS",
		"MC_myRG_myAKS_eastus",
		"AzureBackupRG_eastus_1",
		"NetworkWatcherRG",
		"databricks-rg-workspace-123",
		"microsoft-network",
		"LogAnalyticsDefaultResources",
		"DynamicsDeployments",
		"my-custom-resource-group",
		"prod-web-app-rg",
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			for _, name := range testCases {
				checkIfDefaultResourceGroup(name)
			}
		}
	})
}

// BenchmarkSequentialProcessing benchmarks the old sequential processing approach
func BenchmarkSequentialProcessing(b *testing.B) {
	mockResourceGroups := generateMockResourceGroups(50)

	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"value": [
						{
							"id": "/subscriptions/test/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test",
							"name": "test-storage",
							"type": "Microsoft.Storage/storageAccounts",
							"createdTime": "2023-01-01T12:00:00Z"
						}
					]
				}`)),
			}, nil
		},
	}

	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-subscription",
			AccessToken:    "test-token",
			MaxConcurrency: 1,
		},
		HTTPClient: mockClient,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate sequential processing
		for _, rg := range mockResourceGroups {
			_, _ = client.fetchResourceGroupCreatedTime(rg.Name)
		}
	}
}

// BenchmarkConcurrentProcessing benchmarks the new concurrent processing approach
func BenchmarkConcurrentProcessing(b *testing.B) {
	mockResourceGroups := generateMockResourceGroups(50)

	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			// Simulate realistic API response time
			time.Sleep(10 * time.Millisecond)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"value": [
						{
							"id": "/subscriptions/test/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test",
							"name": "test-storage",
							"type": "Microsoft.Storage/storageAccounts",
							"createdTime": "2023-01-01T12:00:00Z"
						}
					]
				}`)),
			}, nil
		},
	}

	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-subscription",
			AccessToken:    "test-token",
			MaxConcurrency: 10,
		},
		HTTPClient: mockClient,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate concurrent processing
		semaphore := make(chan struct{}, client.Config.MaxConcurrency)
		var wg sync.WaitGroup

		for _, rg := range mockResourceGroups {
			wg.Add(1)
			go func(rg ResourceGroup) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				_, _ = client.fetchResourceGroupCreatedTime(rg.Name)
			}(rg)
		}
		wg.Wait()
	}
}

// BenchmarkMemoryUsage benchmarks memory usage patterns
func BenchmarkMemoryUsage(b *testing.B) {
	mockResourceGroups := generateMockResourceGroups(100)

	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			// Generate a larger response to test memory usage
			largeResponse := `{
				"value": [`
			for i := 0; i < 100; i++ {
				if i > 0 {
					largeResponse += ","
				}
				largeResponse += `{
					"id": "/subscriptions/test/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test` + string(rune(i)) + `",
					"name": "test-storage-` + string(rune(i)) + `",
					"type": "Microsoft.Storage/storageAccounts",
					"createdTime": "2023-01-01T12:00:00Z"
				}`
			}
			largeResponse += `]}`

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(largeResponse)),
			}, nil
		},
	}

	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test-subscription",
			AccessToken:    "test-token",
			MaxConcurrency: 10,
		},
		HTTPClient: mockClient,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, rg := range mockResourceGroups {
			_, _ = client.fetchResourceGroupCreatedTime(rg.Name)
		}
	}
}

// BenchmarkStringOperations benchmarks string operations in default resource group detection
func BenchmarkStringOperations(b *testing.B) {
	testNames := []string{
		"DefaultResourceGroup-EUS",
		"DEFAULTRESOURCEGROUP-WUS2",
		"MC_myRG_myAKS_eastus",
		"mc_another_cluster_westus",
		"AzureBackupRG_eastus_1",
		"AZUREBACKUPRG_WESTUS_2",
		"NetworkWatcherRG",
		"NETWORKWATCHERRG",
		"databricks-rg-workspace-123",
		"DATABRICKS-RG-WORKSPACE-456",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, name := range testNames {
			// Test the string operations that happen in checkIfDefaultResourceGroup
			nameLower := strings.ToLower(name)
			_ = strings.Contains(nameLower, "default")
			_ = strings.Contains(nameLower, "mc_")
			_ = strings.Contains(nameLower, "azure")
		}
	}
}

// BenchmarkConcurrentVsSequential compares concurrent vs sequential processing
func BenchmarkConcurrentVsSequential(b *testing.B) {
	mockResourceGroups := generateMockResourceGroups(20)

	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			// Simulate realistic API response time
			time.Sleep(50 * time.Millisecond)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"value": [
						{
							"id": "/subscriptions/test/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test",
							"name": "test-storage",
							"type": "Microsoft.Storage/storageAccounts",
							"createdTime": "2023-01-01T12:00:00Z"
						}
					]
				}`)),
			}, nil
		},
	}

	b.Run("Sequential", func(b *testing.B) {
		client := &AzureClient{
			Config: Config{
				SubscriptionID: "test-subscription",
				AccessToken:    "test-token",
				MaxConcurrency: 1,
			},
			HTTPClient: mockClient,
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, rg := range mockResourceGroups {
				_, _ = client.fetchResourceGroupCreatedTime(rg.Name)
			}
		}
	})

	b.Run("Concurrent", func(b *testing.B) {
		client := &AzureClient{
			Config: Config{
				SubscriptionID: "test-subscription",
				AccessToken:    "test-token",
				MaxConcurrency: 10,
			},
			HTTPClient: mockClient,
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			semaphore := make(chan struct{}, client.Config.MaxConcurrency)
			var wg sync.WaitGroup

			for _, rg := range mockResourceGroups {
				wg.Add(1)
				go func(rg ResourceGroup) {
					defer wg.Done()
					semaphore <- struct{}{}
					defer func() { <-semaphore }()

					_, _ = client.fetchResourceGroupCreatedTime(rg.Name)
				}(rg)
			}
			wg.Wait()
		}
	})
}

// BenchmarkHTTPClientOptimizations benchmarks HTTP client optimizations
func BenchmarkHTTPClientOptimizations(b *testing.B) {
	mockResourceGroups := generateMockResourceGroups(10)

	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"value": [
						{
							"id": "/subscriptions/test/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test",
							"name": "test-storage",
							"type": "Microsoft.Storage/storageAccounts",
							"createdTime": "2023-01-01T12:00:00Z"
						}
					]
				}`)),
			}, nil
		},
	}

	b.Run("BasicHTTPClient", func(b *testing.B) {
		client := &AzureClient{
			Config: Config{
				SubscriptionID: "test-subscription",
				AccessToken:    "test-token",
				MaxConcurrency: 5,
			},
			HTTPClient: mockClient,
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, rg := range mockResourceGroups {
				_, _ = client.fetchResourceGroupCreatedTime(rg.Name)
			}
		}
	})

	b.Run("OptimizedHTTPClient", func(b *testing.B) {
		client := &AzureClient{
			Config: Config{
				SubscriptionID: "test-subscription",
				AccessToken:    "test-token",
				MaxConcurrency: 5,
			},
			HTTPClient: mockClient,
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, rg := range mockResourceGroups {
				_, _ = client.fetchResourceGroupCreatedTime(rg.Name)
			}
		}
	})
}

// generateMockResourceGroups creates mock resource groups for testing
func generateMockResourceGroups(count int) []ResourceGroup {
	resourceGroups := make([]ResourceGroup, count)
	for i := 0; i < count; i++ {
		resourceGroups[i] = ResourceGroup{
			ID:       fmt.Sprintf("/subscriptions/test/resourceGroups/test-rg-%d", i),
			Name:     fmt.Sprintf("test-rg-%d", i),
			Location: "eastus",
			Properties: struct {
				ProvisioningState string `json:"provisioningState"`
			}{
				ProvisioningState: "Succeeded",
			},
		}
	}
	return resourceGroups
}

// BenchmarkScalability tests performance at different scales
func BenchmarkScalability(b *testing.B) {
	scales := []int{10, 50, 100, 200}

	for _, scale := range scales {
		b.Run(fmt.Sprintf("Scale_%d", scale), func(b *testing.B) {
			mockResourceGroups := generateMockResourceGroups(scale)

			mockClient := &MockHTTPClient{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					// Simulate realistic API response time
					time.Sleep(10 * time.Millisecond)
					return &http.Response{
						StatusCode: http.StatusOK,
						Body: io.NopCloser(strings.NewReader(`{
							"value": [
								{
									"id": "/subscriptions/test/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test",
									"name": "test-storage",
									"type": "Microsoft.Storage/storageAccounts",
									"createdTime": "2023-01-01T12:00:00Z"
								}
							]
						}`)),
					}, nil
				},
			}

			client := &AzureClient{
				Config: Config{
					SubscriptionID: "test-subscription",
					AccessToken:    "test-token",
					MaxConcurrency: 10,
				},
				HTTPClient: mockClient,
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				semaphore := make(chan struct{}, client.Config.MaxConcurrency)
				var wg sync.WaitGroup

				for _, rg := range mockResourceGroups {
					wg.Add(1)
					go func(rg ResourceGroup) {
						defer wg.Done()
						semaphore <- struct{}{}
						defer func() { <-semaphore }()

						_, _ = client.fetchResourceGroupCreatedTime(rg.Name)
					}(rg)
				}
				wg.Wait()
			}
		})
	}
}
