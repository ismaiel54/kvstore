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

# Run 3 nodes locally (placeholder for Phase 7)
run-3:
	@echo "Starting 3-node cluster..."
	@echo "Node 1: localhost:50051"
	@go run ./cmd/kvstore --node-id=node1 --listen=:50051 &
	@echo "Node 2: localhost:50052"
	@go run ./cmd/kvstore --node-id=node2 --listen=:50052 --seed=localhost:50051 &
	@echo "Node 3: localhost:50053"
	@go run ./cmd/kvstore --node-id=node3 --listen=:50053 --seed=localhost:50051 &
	@echo "Cluster started. Press Ctrl+C to stop all nodes."
	@wait

# Clean generated files
clean:
	rm -rf internal/gen
	go clean

