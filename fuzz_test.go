package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// FuzzValidateConcurrency ensures validateConcurrency never returns less than 1.
func FuzzValidateConcurrency(f *testing.F) {
	testcases := []int{0, 1, -5, 10}
	for _, tc := range testcases {
		f.Add(tc)
	}
	f.Fuzz(func(t *testing.T, n int) {
		if res := validateConcurrency(n); res < 1 {
			t.Fatalf("validateConcurrency(%d) returned %d", n, res)
		}
	})
}

// FuzzCheckIfDefaultResourceGroup ensures function handles arbitrary names.
func FuzzCheckIfDefaultResourceGroup(f *testing.F) {
	seeds := []string{"DefaultResourceGroup-EUS", "MC_rg_aks_eastus", "custom"}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		_ = checkIfDefaultResourceGroup(name)
	})
}

// FuzzMakeAzureRequest validates error handling for various status codes.
func FuzzMakeAzureRequest(f *testing.F) {
	statusCodes := []int{200, 400, 401, 404, 500}
	for _, sc := range statusCodes {
		f.Add(sc)
	}
	f.Fuzz(func(t *testing.T, code int) {
		if code < 100 || code > 599 {
			return
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			fmt.Fprint(w, `{"value": []}`)
		}))
		defer server.Close()

		client := &AzureClient{Config: Config{AccessToken: "token"}, HTTPClient: server.Client()}
		resp, err := client.makeAzureRequest(server.URL)

		if code == http.StatusOK {
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected status 200, got %d", resp.StatusCode)
			}
		} else {
			if err == nil {
				t.Fatalf("expected error for status %d", code)
			}
		}
		if resp != nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	})
}
