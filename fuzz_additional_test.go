package main

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestSpinnerStartStop verifies that the spinner outputs frames and stops correctly.
func TestSpinnerStartStop(t *testing.T) {
	spinner := NewSpinner("testing spinner")

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	spinner.Start()
	time.Sleep(350 * time.Millisecond)
	spinner.Stop()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Count spinner frame updates via carriage returns
	frameCount := strings.Count(output, "\r")
	if frameCount < 2 {
		t.Errorf("expected spinner to output multiple frames, got %d", frameCount)
	}
}

// TestRateLimitingFuzz runs multiple iterations with random concurrency and delays
// to ensure semaphore based rate limiting works under varied conditions.
func TestRateLimitingFuzz(t *testing.T) {
	rand.Seed(42)
	for i := 0; i < 5; i++ {
		concurrency := rand.Intn(5) + 1
		groupCount := rand.Intn(5) + 1

		t.Run(fmt.Sprintf("c%d_g%d", concurrency, groupCount), func(t *testing.T) {
			rgs := generateMockResourceGroups(groupCount)
			var concurrent int32
			var maxConcurrent int32
			mockClient := &MockHTTPClient{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					cur := atomic.AddInt32(&concurrent, 1)
					for {
						mc := atomic.LoadInt32(&maxConcurrent)
						if cur > mc {
							if atomic.CompareAndSwapInt32(&maxConcurrent, mc, cur) {
								break
							}
						} else {
							break
						}
					}
					// Simulate varied network latency
					time.Sleep(time.Duration(rand.Intn(20)+5) * time.Millisecond)
					atomic.AddInt32(&concurrent, -1)
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"value": []}`)),
					}, nil
				},
			}
			client := &AzureClient{
				Config: Config{
					SubscriptionID: "test-sub",
					AccessToken:    "test-token",
					MaxConcurrency: concurrency,
					Porcelain:      true,
				},
				HTTPClient: mockClient,
			}

			start := time.Now()
			client.processResourceGroupsConcurrently(rgs)
			duration := time.Since(start)

			if maxConcurrent > int32(concurrency) {
				t.Errorf("observed max concurrency %d > %d", maxConcurrent, concurrency)
			}
			if duration > time.Second {
				t.Errorf("processing took too long: %v", duration)
			}
		})
	}
}
