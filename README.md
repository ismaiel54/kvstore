# Distributed Key-Value Store

A fault-tolerant distributed key-value store inspired by Dynamo/Bigtable, built in Go.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Client                                │
└───────────────────────┬─────────────────────────────────────┘
                        │ gRPC
                        │
        ┌───────────────┼───────────────┐
        │               │               │
   ┌────▼────┐     ┌────▼────┐     ┌────▼────┐
   │ Node 1  │     │ Node 2  │     │ Node 3  │
   │         │     │         │     │         │
   │ ┌─────┐ │     │ ┌─────┐ │     │ ┌─────┐ │
   │ │Ring │ │     │ │Ring │ │     │ │Ring │ │
   │ └──┬──┘ │     │ └──┬──┘ │     │ └──┬──┘ │
   │    │    │     │    │    │     │    │    │
   │ ┌──▼──┐ │     │ ┌──▼──┐ │     │ ┌──▼──┐ │
   │ │Store│ │     │ │Store│ │     │ │Store│ │
   │ └─────┘ │     │ └─────┘ │     │ └─────┘ │
   └─────────┘     └─────────┘     └─────────┘
        │               │               │
        └───────────────┼───────────────┘
                        │
                   Gossip Protocol
```

### Components

- **gRPC API**: Protocol buffer-based API for Put/Get/Delete operations
- **Vector Clocks**: Track causality and detect conflicts
- **Consistent Hashing**: Distribute keys across nodes (Phase 2)
- **Replication**: N replicas per key with quorum reads/writes (Phase 3)
- **Gossip Membership**: SWIM-style failure detection (Phase 5)
- **Read Repair**: Automatically fix stale replicas (Phase 6)

## Current Status: Phase 1 Complete ✅

### Implemented
- ✅ gRPC API with Put/Get/Delete operations
- ✅ Single-node in-memory storage
- ✅ Vector clock implementation with conflict detection
- ✅ Basic unit tests for clock and storage

### Not Yet Implemented
- ⏳ Consistent hashing ring (Phase 2)
- ⏳ Replication and quorum coordination (Phase 3)
- ⏳ Conflict resolution strategies (Phase 4)
- ⏳ Gossip membership protocol (Phase 5)
- ⏳ Read repair (Phase 6)
- ⏳ Multi-node cluster runner (Phase 7)

## Prerequisites

- Go 1.21 or later
- `protoc` (Protocol Buffer compiler)
- `protoc-gen-go` and `protoc-gen-go-grpc` plugins

### Installing Prerequisites

```bash
# Install protoc (macOS)
brew install protobuf

# Install Go plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Ensure plugins are in PATH
export PATH=$PATH:$(go env GOPATH)/bin
```

## Building

```bash
# Generate protobuf code
make proto

# Run tests
make test

# Build the binary
go build ./cmd/kvstore
```

## Running

### Start a Single Node

```bash
# Default: node1 on :50051
make run

# Or with custom options
go run ./cmd/kvstore --node-id=node1 --listen=:50051
```

### Start Multiple Nodes (Phase 7)

```bash
# Will be available in Phase 7
make run-3
```

## Usage Examples

### Using grpcurl

Install grpcurl:
```bash
brew install grpcurl  # macOS
# or
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

### Put a Value

```bash
grpcurl -plaintext -d '{
  "key": "mykey",
  "value": "SGVsbG8gV29ybGQ=",
  "client_id": "client1",
  "request_id": "req1"
}' localhost:50051 kvstore.KVStore/Put
```

Response:
```json
{
  "status": "SUCCESS",
  "version": {
    "entries": [
      {
        "nodeId": "node1",
        "counter": 1
      }
    ]
  }
}
```

### Get a Value

```bash
grpcurl -plaintext -d '{
  "key": "mykey",
  "client_id": "client1",
  "request_id": "req2"
}' localhost:50051 kvstore.KVStore/Get
```

Response:
```json
{
  "status": "SUCCESS",
  "value": {
    "value": "SGVsbG8gV29ybGQ=",
    "version": {
      "entries": [
        {
          "nodeId": "node1",
          "counter": 1
        }
      ]
    }
  }
}
```

### Delete a Value

```bash
grpcurl -plaintext -d '{
  "key": "mykey",
  "client_id": "client1",
  "request_id": "req3"
}' localhost:50051 kvstore.KVStore/Delete
```

### Using Base64 for Binary Values

```bash
# Encode value to base64
echo -n "Hello World" | base64
# Output: SGVsbG8gV29ybGQ=

# Use in grpcurl
grpcurl -plaintext -d '{
  "key": "test",
  "value": "SGVsbG8gV29ybGQ="
}' localhost:50051 kvstore.KVStore/Put
```

## Consistency Model

### Current (Phase 1)
- Single-node: Strong consistency
- All operations are immediately visible

### Future (Phase 3+)
- **Tunable Consistency**: Configurable R (read quorum) and W (write quorum)
- **Eventual Consistency**: With R + W < N, system allows stale reads
- **Strong Consistency**: With R + W > N, guarantees linearizability
- **Conflict Resolution**: Vector clocks detect concurrent writes; conflicts returned to client

## Vector Clocks

Vector clocks track causality in distributed operations:

- Each node maintains a counter per node ID
- On write: node increments its own counter
- On read: returns value with its vector clock
- Comparison:
  - **Before/After**: One clock dominates (happened-before relationship)
  - **Concurrent**: Clocks are incomparable (concurrent writes)

Example:
```
Node1: {node1: 2, node2: 1}  → After
Node2: {node1: 1, node2: 1}  → Before
```

## Project Structure

```
kvstore/
├── api/
│   └── kvstore.proto          # gRPC service definitions
├── cmd/
│   └── kvstore/
│       └── main.go            # CLI entrypoint
├── internal/
│   ├── gen/
│   │   └── api/               # Generated protobuf code
│   ├── clock/                 # Vector clock implementation
│   ├── node/                  # gRPC server and node lifecycle
│   ├── storage/               # Local KV storage (in-memory)
│   ├── ring/                  # Consistent hashing (Phase 2)
│   ├── replication/           # Replica selection (Phase 3)
│   ├── quorum/                # Quorum coordination (Phase 3)
│   ├── gossip/                # Membership protocol (Phase 5)
│   ├── repair/                # Read repair (Phase 6)
│   └── config/                # Configuration parsing
├── pkg/
│   └── types/                 # Shared types
├── scripts/                   # Cluster runner scripts
├── Makefile                   # Build targets
└── README.md                  # This file
```

## Development

### Running Tests

```bash
# All tests
make test

# Specific package
go test ./internal/clock
go test ./internal/storage
```

### Code Generation

```bash
# Regenerate protobuf code
make proto
```

### Code Quality

- All code must compile: `go build ./...`
- All tests must pass: `go test ./...`
- Files should be under ~300 lines
- Each package has documentation

## Limitations (Phase 1)

1. **Single Node Only**: No distributed features yet
2. **In-Memory Only**: Data is lost on restart
3. **No Persistence**: No write-ahead log or snapshot
4. **No Replication**: Single copy of data
5. **No Failure Handling**: Node failures cause data loss
6. **No Conflict Resolution UI**: Conflicts detected but not resolved automatically

## Roadmap

- **Phase 2**: Consistent hashing ring + node routing
- **Phase 3**: Replication + quorum reads/writes
- **Phase 4**: Conflict resolution strategies
- **Phase 5**: Gossip membership + failure detection
- **Phase 6**: Read repair
- **Phase 7**: Multi-node cluster runner + demo

## License

MIT

