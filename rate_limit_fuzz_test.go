package main

import (
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fuzz-like test to ensure semaphore respects max concurrency with varied delays
func TestRateLimitingFuzz(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < 5; i++ { // multiple iterations with random parameters
		rgCount := rand.Intn(5) + 5 // 5-9 resource groups
		maxConc := rand.Intn(4) + 1 // 1-4 concurrency

		resourceGroups := make([]ResourceGroup, rgCount)
		for j := range resourceGroups {
			resourceGroups[j] = ResourceGroup{Name: "rg" + strconv.Itoa(j)}
		}

		var concurrent int32
		var maxObserved int32
		mockClient := &MockHTTPClient{DoFunc: func(req *http.Request) (*http.Response, error) {
			atomic.AddInt32(&concurrent, 1)
			c := atomic.LoadInt32(&concurrent)
			for {
				m := atomic.LoadInt32(&maxObserved)
				if c > m {
					if atomic.CompareAndSwapInt32(&maxObserved, m, c) {
						break
					}
				} else {
					break
				}
			}
			time.Sleep(time.Duration(rand.Intn(20)+5) * time.Millisecond)
			atomic.AddInt32(&concurrent, -1)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"value":[]}`))}, nil
		}}

		client := &AzureClient{Config: Config{SubscriptionID: "x", AccessToken: "y", MaxConcurrency: maxConc, Porcelain: true}, HTTPClient: mockClient}
		client.processResourceGroupsConcurrently(resourceGroups)

		if int(maxObserved) > maxConc {
			t.Fatalf("iteration %d: observed concurrency %d > limit %d", i, maxObserved, maxConc)
		}
	}
}
