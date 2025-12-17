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

## Current Status: Phase 3 Complete

### Implemented
- gRPC API with Put/Get/Delete operations
- Single-node in-memory storage
- Vector clock implementation with conflict detection
- Consistent hashing ring with virtual nodes
- Request routing to responsible nodes
- Static membership via configuration
- **Replication factor N (default 3)**
- **Quorum reads R and writes W (defaults: R=2, W=2)**
- **Quorum coordinator with parallel fanout**
- **Internal RPC service for replica operations**
- **Version reconciliation and conflict detection**
- Multi-node cluster runner with configurable N/R/W
- Comprehensive unit tests for quorum, replication, storage

### Not Yet Implemented
- Conflict resolution strategies (Phase 4)
- Gossip membership protocol (Phase 5)
- Read repair (Phase 6)

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

### Start Multiple Nodes

```bash
# Start 3-node cluster
make run-3

# Or manually:
./scripts/run_local_cluster.sh
```

The cluster will start 3 nodes:
- Node 1: `localhost:50051`
- Node 2: `localhost:50052`
- Node 3: `localhost:50053`

All nodes know about each other via the `--peers` flag.

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

## Phase 2: Sharding via Consistent Hashing

Phase 2 adds distributed key routing using consistent hashing:

- **Consistent Hashing Ring**: Keys are distributed across nodes using a hash ring with virtual nodes (default 128 per physical node)
- **Request Routing**: Any node can act as coordinator and routes requests to the responsible node
- **Static Membership**: Nodes are configured via `--peers` flag (no dynamic discovery yet)
- **Deterministic**: Same nodes produce same key-to-node mapping

### Example: Routing

```bash
# Start 3-node cluster
make run-3

# In another terminal, put a key to any node
grpcurl -plaintext -d '{
  "key": "user:123",
  "value": "SGVsbG8gV29ybGQ=",
  "client_id": "client1",
  "request_id": "req1"
}' localhost:50051 kvstore.KVStore/Put

# The request will be routed to the responsible node based on the key hash
# Check logs to see which node actually handled the request
```

### Configuration

```bash
# Single node (no routing)
go run ./cmd/kvstore --node-id=n1 --listen=:50051

# Multi-node with peers
go run ./cmd/kvstore \
  --node-id=n1 \
  --listen=:50051 \
  --peers="n2=127.0.0.1:50052,n3=127.0.0.1:50053" \
  --vnodes=128
```

## Phase 3: Replication and Quorum

Phase 3 adds replication and quorum-based consistency:

- **Replication Factor N**: Each key is replicated to N nodes (default 3)
- **Quorum Reads (R)**: Read requires R successful responses (default 2)
- **Quorum Writes (W)**: Write requires W successful acknowledgements (default 2)
- **Parallel Fanout**: Coordinator sends requests to all replicas in parallel
- **Early Termination**: Returns success when quorum is met (doesn't wait for all replicas)
- **Version Reconciliation**: Detects conflicts when concurrent versions exist
- **Fault Tolerance**: System continues operating with R=2, W=2 even if 1 node fails

### Example: Quorum Behavior

```bash
# Start 3-node cluster with RF=3, R=2, W=2
make run-3

# Put a key (requires W=2 acks from 3 replicas)
grpcurl -plaintext -d '{
  "key": "user:123",
  "value": "SGVsbG8gV29ybGQ=",
  "consistency_w": 2,
  "client_id": "test",
  "request_id": "req1"
}' localhost:50051 kvstore.KVStore/Put

# Get the key (requires R=2 responses from 3 replicas)
grpcurl -plaintext -d '{
  "key": "user:123",
  "consistency_r": 2,
  "client_id": "test",
  "request_id": "req2"
}' localhost:50052 kvstore.KVStore/Get

# Kill one node (e.g., node 3)
pkill -f "node-id=n3"

# System still works! Get/Put succeed with R=2, W=2 from remaining 2 nodes
grpcurl -plaintext -d '{
  "key": "user:123",
  "consistency_r": 2,
  "client_id": "test",
  "request_id": "req3"
}' localhost:50051 kvstore.KVStore/Get
```

### Configuration

```bash
# Start node with custom replication settings
go run ./cmd/kvstore \
  --node-id=n1 \
  --listen=:50051 \
  --peers="n2=127.0.0.1:50052,n3=127.0.0.1:50053" \
  --rf=3 \
  --r=2 \
  --w=2

# Or use environment variables in runner script
RF=3 R=2 W=2 make run-3
```

## Limitations (Phase 3)

1. **Static Membership**: Nodes must be configured manually; no dynamic discovery
2. **In-Memory Only**: Data is lost on restart
3. **No Persistence**: No write-ahead log or snapshot
4. **No Automatic Conflict Resolution**: Conflicts detected but client must resolve
5. **No Read Repair**: Stale replicas are not automatically repaired
6. **No Hinted Handoff**: Writes fail if replica is down (no buffering)

## Roadmap

- **Phase 2**: Consistent hashing ring + node routing (COMPLETE)
- **Phase 3**: Replication + quorum reads/writes (COMPLETE)
- **Phase 4**: Conflict resolution strategies
- **Phase 5**: Gossip membership + failure detection
- **Phase 6**: Read repair

## License

MIT
