MODULE       := otel-helper-go
VERSION_FILE := ../VERSION
VERSION      := $(shell if [ -f $(VERSION_FILE) ]; then cat $(VERSION_FILE); else echo dev; fi)
LDFLAGS      := -s -w -X main.injectedVersion=$(VERSION)
DIST     := dist

.PHONY: build build-all test test-coverage lint vet clean size-report

# Default: build for current platform
build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(DIST)/otel-helper .

# Cross-compile for all 5 platforms
build-all: clean
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/otel-helper-macos-arm64     .
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/otel-helper-macos-intel     .
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/otel-helper-linux-x64       .
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/otel-helper-linux-arm64     .
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/otel-helper-windows.exe     .

test:
	go test ./... -v

test-coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out
	@echo ""
	@echo "To view HTML report: go tool cover -html=coverage.out"

lint:
	@which golangci-lint > /dev/null 2>&1 || echo "golangci-lint not installed, running go vet only"
	@which golangci-lint > /dev/null 2>&1 && golangci-lint run ./... || true
	go vet ./...

vet:
	go vet ./...

clean:
	rm -rf $(DIST) coverage.out

size-report: build-all
	@echo ""
	@echo "=== Binary Size Report ==="
	@ls -lh $(DIST)/ | grep -v "^total"
	@echo ""
	@echo "Compare with Python PyInstaller binaries (typically 25-28 MB)"
