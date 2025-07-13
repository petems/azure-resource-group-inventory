
# azure-resource-group-inventory Makefile

# Binary name
BINARY_NAME=azrginventory

# Build the application
build:
	go build -o $(BINARY_NAME) .

# Clean build artifacts
clean:
	go clean
	rm -f $(BINARY_NAME)

# Run tests
test:
	go test -v ./...

# Download dependencies
deps:
	go mod tidy
	go mod download

# Build for multiple platforms
build-all:
	GOOS=linux GOARCH=amd64 go build -o $(BINARY_NAME)-linux-amd64 .
	GOOS=windows GOARCH=amd64 go build -o $(BINARY_NAME)-windows-amd64.exe .
	GOOS=darwin GOARCH=amd64 go build -o $(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -o $(BINARY_NAME)-darwin-arm64 .

# Install the binary to GOPATH/bin
install:
	go install .

# Run the application with example parameters
run:
	./$(BINARY_NAME) --help

# Format code
fmt:
	go fmt ./...

# Run linter
lint:
	golangci-lint run

# Show help
help:
	@echo "Available commands:"
	@echo "  build      - Build the application"
	@echo "  clean      - Clean build artifacts"
	@echo "  test       - Run tests"
	@echo "  deps       - Download dependencies"
	@echo "  build-all  - Build for multiple platforms"
	@echo "  install    - Install to GOPATH/bin"
	@echo "  run        - Run the application (shows help)"
	@echo "  fmt        - Format code"
	@echo "  lint       - Run linter"
	@echo "  help       - Show this help"

.PHONY: build clean test deps build-all install run fmt lint help