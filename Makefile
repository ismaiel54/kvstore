.PHONY: proto proto-check fmt vet lint test test-short smoke ci clean

# Protobuf generation
proto:
	@echo "Generating protobuf code..."
	@mkdir -p internal/gen
	@PATH=$$(go env GOPATH)/bin:$$PATH protoc --go_out=internal/gen --go_opt=paths=source_relative \
		--go-grpc_out=internal/gen --go-grpc_opt=paths=source_relative \
		api/kvstore.proto

# Format code
fmt:
	@echo "Formatting Go code..."
	@gofmt -w $$(find . -name '*.go' -not -path './internal/gen/*' -not -path './.local/*')

# Check formatting
fmt-check:
	@echo "Checking code formatting..."
	@if [ $$(gofmt -l $$(find . -name '*.go' -not -path './internal/gen/*' -not -path './.local/*') | wc -l) -ne 0 ]; then \
		echo "Error: Code is not formatted. Run 'make fmt' to fix."; \
		exit 1; \
	fi

# Run go vet
vet:
	@echo "Running go vet..."
	@go vet ./...

# Run linter
lint:
	@echo "Running golangci-lint..."
	@if ! command -v golangci-lint > /dev/null; then \
		echo "Error: golangci-lint is not installed."; \
		echo "Install it with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		echo "Or visit: https://golangci-lint.run/usage/install/"; \
		exit 1; \
	fi
	@golangci-lint run

# Run tests with race detector
test:
	@echo "Running tests with race detector..."
	@go test ./... -race -count=1

# Run short tests only (unit tests, no integration)
test-short:
	@echo "Running short tests (unit tests only)..."
	@go test ./... -short -race -count=1

# Run smoke tests (integration tests)
smoke:
	@echo "Running smoke tests (integration tests)..."
	@go test ./internal/it/... -v -timeout 5m

# Check if proto files are up-to-date
proto-check: proto
	@echo "Checking if generated protobuf code is up-to-date..."
	@if [ -n "$$(git diff --name-only internal/gen 2>/dev/null)" ]; then \
		echo "Error: Generated protobuf code is out of date. Run 'make proto' and commit the changes."; \
		git diff internal/gen; \
		exit 1; \
	fi
	@echo "Protobuf code is up-to-date."

# CI pipeline: runs all quality checks
ci: fmt-check vet lint test smoke
	@echo "All CI checks passed!"

# Run a single node
run:
	go run ./cmd/kvstore

# Run 3 nodes locally
run-3:
	@./scripts/cluster.sh up 3

# Cluster management
cluster-up:
	@./scripts/cluster.sh up

cluster-down:
	@./scripts/cluster.sh down

cluster-status:
	@./scripts/cluster.sh status

# Demos
demo-quorum:
	@./scripts/demo_quorum.sh

demo-conflict:
	@./scripts/demo_conflict.sh

demo-repair:
	@./scripts/demo_read_repair.sh

demo-membership:
	@./scripts/demo_membership.sh

# Clean generated files
clean:
	rm -rf internal/gen
	go clean

