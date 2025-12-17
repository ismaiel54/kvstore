.PHONY: proto test run run-3 clean

# Protobuf generation
proto:
	@echo "Generating protobuf code..."
	@mkdir -p internal/gen
	@PATH=$$(go env GOPATH)/bin:$$PATH protoc --go_out=internal/gen --go_opt=paths=source_relative \
		--go-grpc_out=internal/gen --go-grpc_opt=paths=source_relative \
		api/kvstore.proto

# Run tests
test:
	go test ./...

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

