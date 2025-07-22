func TestFetchStorageAccounts(t *testing.T) {
	// Create a mock HTTP client
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			// Check the URL to determine which response to return
			if strings.Contains(req.URL.Path, "/providers/Microsoft.Storage/storageAccounts") && !strings.Contains(req.URL.Path, "/test-storage") {
				// Return list of storage accounts
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"value": [
							{
								"id": "/subscriptions/test/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test-storage",
								"name": "test-storage",
								"location": "eastus",
								"type": "Microsoft.Storage/storageAccounts",
								"properties": {
									"provisioningState": "Succeeded",
									"accountType": "Standard_LRS",
									"primaryEndpoints": {
										"blob": "https://test-storage.blob.core.windows.net/",
										"queue": "https://test-storage.queue.core.windows.net/",
										"table": "https://test-storage.table.core.windows.net/",
										"file": "https://test-storage.file.core.windows.net/"
									}
								}
							}
						]
					}`)),
				}
				return resp, nil
			} else if strings.Contains(req.URL.Path, "/test-storage") {
				// Return individual storage account details
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"id": "/subscriptions/test/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test-storage",
						"name": "test-storage",
						"location": "eastus",
						"type": "Microsoft.Storage/storageAccounts",
						"properties": {
							"provisioningState": "Succeeded",
							"accountType": "Standard_LRS"
						}
					}`)),
				}
				return resp, nil
			} else if strings.Contains(req.URL.Path, "/resourceGroups/test-rg/resources") {
				// Return resource group resources with creation time
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"value": [
							{
								"id": "/subscriptions/test/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test-storage",
								"name": "test-storage",
								"type": "Microsoft.Storage/storageAccounts",
								"createdTime": "2023-01-01T12:00:00Z"
							}
						]
					}`)),
				}
				return resp, nil
			}
			
			// Default response
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{"value": []}`)),
			}
			return resp, nil
		},
	}

	client := &AzureClient{
		Config: Config{
			SubscriptionID: "test",
			AccessToken: "test-token",
			MaxConcurrency: 1,
		},
		HTTPClient: mockClient,
	}

	// Capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := client.FetchStorageAccounts()

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if !strings.Contains(output, "Found 1 storage accounts") {
		t.Errorf("Expected output to contain 'Found 1 storage accounts', got: %s", output)
	}

	if !strings.Contains(output, "test-storage") {
		t.Errorf("Expected output to contain storage account name 'test-storage', got: %s", output)
	}

	if !strings.Contains(output, "Standard_LRS") {
		t.Errorf("Expected output to contain account type 'Standard_LRS', got: %s", output)
	}
}

func TestPrintResourceGroupResultWithResources_Porcelain(t *testing.T) {
	ac := &AzureClient{Config: Config{Porcelain: true}}

	created1, err := time.Parse(time.RFC3339, "2023-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("failed to parse time: %v", err)
	}
	created2, err := time.Parse(time.RFC3339, "2023-02-01T00:00:00Z")
	if err != nil {
		t.Fatalf("failed to parse time: %v", err)
	}

	resources := []Resource{
		{Name: "res1", Type: "type1", CreatedTime: &created1},
		{Name: "res2", Type: "type2", CreatedTime: &created2},
	}

	rg := ResourceGroup{Name: "my-rg", Location: "eastus", Properties: struct {
		ProvisioningState string `json:"provisioningState"`
	}{ProvisioningState: "Succeeded"}}
	result := ResourceGroupResult{ResourceGroup: rg}

	old := os.Stdout
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("failed to create pipe: %v", pipeErr)
	}
	os.Stdout = w

	ac.printResourceGroupResultWithResources(result, resources)

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close pipe writer: %v", err)
	}
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed to read output: %v", err)
	}

	expected := fmt.Sprintf("%s\t%s\t%s\t%s\tfalse\n", rg.Name, rg.Location, rg.Properties.ProvisioningState, created1.Format(time.RFC3339))
	if strings.TrimSpace(buf.String()) != strings.TrimSpace(expected) {
		t.Errorf("unexpected output:\n%s", buf.String())
	}
}

func TestPrintResourceGroupResultWithResources_Human(t *testing.T) {
	ac := &AzureClient{Config: Config{Porcelain: false}}

	rg := ResourceGroup{Name: "networkwatcherrg", Location: "eastus", Properties: struct {
		ProvisioningState string `json:"provisioningState"`
	}{ProvisioningState: "Succeeded"}}
	result := ResourceGroupResult{ResourceGroup: rg}

	old := os.Stdout
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("failed to create pipe: %v", pipeErr)
	}
	os.Stdout = w

	ac.printResourceGroupResultWithResources(result, nil)

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close pipe writer: %v", err)
	}
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	output := buf.String()

	if !strings.Contains(output, "DEFAULT RESOURCE GROUP DETECTED") {
		t.Errorf("expected default detection in output, got:\n%s", output)
	}
	if !strings.Contains(output, "No resources found") {
		t.Errorf("expected no resources message, got:\n%s", output)
	}
}