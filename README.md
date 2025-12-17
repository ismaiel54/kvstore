# kvstore

A fault-tolerant distributed key-value store inspired by Amazon Dynamo, built in Go. This project implements core distributed systems concepts including consistent hashing, quorum-based replication, vector clocks for conflict detection, gossip-based membership, and read repair.

## Features

- **Consistent Hashing**: Virtual nodes for even key distribution
- **Replication**: Configurable replication factor N (default: 3)
- **Quorum Consistency**: Tunable read (R) and write (W) quorums
- **Vector Clocks**: Causality tracking and conflict detection
- **Conflict Resolution**: Returns siblings for concurrent writes
- **Gossip Membership**: SWIM-style failure detection
- **Read Repair**: Automatic anti-entropy via reads
- **gRPC API**: Protocol buffer-based client interface
- **Operability**: Health checks, membership queries, ring inspection

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Client                                │
└───────────────────────┬─────────────────────────────────────┘
                        │ gRPC (Put/Get/Delete)
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
   └────┬────┘     └────┬────┘     └────┬────┘
        │               │               │
        └───────────────┼───────────────┘
                        │
              Gossip Protocol (Membership)
```

### Request Flow

1. **Client → Coordinator**: Client sends Put/Get/Delete to any node
2. **Ring Lookup**: Coordinator uses consistent hashing to find owner node
3. **Replica Selection**: Coordinator selects N replicas from preference list
4. **Quorum Operation**: 
   - **Write**: Fan out to N replicas, wait for W acks
   - **Read**: Fan out to N replicas, wait for R responses
5. **Reconciliation**: Coordinator reconciles versions using vector clocks
6. **Read Repair**: If stale replicas detected, repair asynchronously
7. **Response**: Return value or conflicts to client

### Components

- **Ring** (`internal/ring/`): Consistent hashing with virtual nodes
- **Storage** (`internal/storage/`): In-memory key-value store with vector clocks
- **Quorum** (`internal/quorum/`): Parallel fanout and quorum coordination
- **Replication** (`internal/replication/`): Replica selection from preference list
- **Repair** (`internal/repair/`): Conflict reconciliation and read repair
- **Gossip** (`internal/gossip/`): SWIM-style membership and failure detection
- **Node** (`internal/node/`): gRPC server, request routing, lifecycle

## Quickstart

### Prerequisites

- Go 1.21+
- `protoc` (Protocol Buffers compiler)
- Go protobuf plugins:
  ```bash
  go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
  ```
- `golangci-lint` (optional, for linting):
  ```bash
  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
  ```
  Or visit: https://golangci-lint.run/usage/install/

### Build

```bash
# Generate protobuf code
make proto

# Run tests
make test

# Build binary
go build -o kvstore ./cmd/kvstore
```

### Quality Gate

The project includes a comprehensive quality gate:

```bash
# Format code
make fmt

# Check formatting
make fmt-check

# Run go vet
make vet

# Run linter (requires golangci-lint)
make lint

# Run unit tests
make test-short

# Run all tests (including integration)
make test

# Run smoke tests (integration tests)
make smoke

# Run full CI pipeline locally
make ci
```

**Quality Checks:**
- `make fmt-check`: Verifies code formatting
- `make vet`: Runs go vet for common errors
- `make lint`: Runs golangci-lint with configured rules
- `make proto-check`: Verifies generated protobuf code is up-to-date
- `make test`: Runs all tests with race detector
- `make smoke`: Runs integration tests (3-node cluster)

### Start Cluster

```bash
# Start 3-node cluster
make cluster-up
# or
./scripts/cluster.sh up 3

# Check status
make cluster-status
# or
./scripts/cluster.sh status

# View logs
./scripts/cluster.sh logs n1

# Stop cluster
make cluster-down
```

## Demos

### Quorum Tolerance

Demonstrates that the system continues operating with 1 node down (R=2, W=2):

```bash
make demo-quorum
```

**Expected Output:**
- Cluster starts with 3 nodes
- Put/Get operations succeed
- Node n3 is killed
- Put/Get still succeed (quorum met with 2 nodes)

### Conflict Detection

Demonstrates concurrent writes creating conflicts returned as siblings:

```bash
make demo-conflict
```

**Expected Output:**
- Two concurrent writes to the same key
- Get returns conflicts (multiple siblings)
- Client can resolve conflicts

### Read Repair

Demonstrates automatic repair of stale replicas:

```bash
make demo-repair
```

**Expected Output:**
- Write initial value
- Kill node n3
- Write new value (n3 becomes stale)
- Restart n3
- Get triggers read repair
- Subsequent Get shows convergence

### Membership

Demonstrates gossip-based failure detection:

```bash
make demo-membership
```

**Expected Output:**
- Query membership (all nodes ALIVE)
- Kill node n2
- Query membership (n2 becomes SUSPECT)
- Wait for timeout
- Query membership (n2 becomes DEAD)

## API

### Put

```bash
grpcurl -plaintext -d '{
  "key": "user:123",
  "value": "SGVsbG8gV29ybGQ=",
  "consistency_w": 2,
  "client_id": "client1",
  "request_id": "req1",
  "version": {
    "entries": [
      {"nodeId": "n1", "counter": 1}
    ]
  }
}' localhost:50051 kvstore.KVStore/Put
```

**Request Fields:**
- `key`: Key to store
- `value`: Base64-encoded value
- `consistency_w`: Write quorum size (optional, uses default)
- `consistency_r`: Read quorum size (optional, for read-modify-write)
- `version`: Optional version context from previous Get (for conflict resolution)
- `client_id`: Client identifier
- `request_id`: Request identifier for tracing

**Response:**
- `status`: SUCCESS or ERROR
- `version`: New vector clock after write

### Get

```bash
grpcurl -plaintext -d '{
  "key": "user:123",
  "consistency_r": 2,
  "client_id": "client1",
  "request_id": "req2"
}' localhost:50051 kvstore.KVStore/Get
```

**Response (Single Winner):**
```json
{
  "status": "SUCCESS",
  "value": {
    "value": "SGVsbG8gV29ybGQ=",
    "version": {
      "entries": [
        {"nodeId": "n1", "counter": 2}
      ]
    },
    "deleted": false
  }
}
```

**Response (Conflicts):**
```json
{
  "status": "SUCCESS",
  "conflicts": [
    {
      "value": "dmFsdWUx",
      "version": {"entries": [{"nodeId": "n1", "counter": 1}]},
      "deleted": false
    },
    {
      "value": "dmFsdWUy",
      "version": {"entries": [{"nodeId": "n2", "counter": 1}]},
      "deleted": false
    }
  ]
}
```

### Delete

```bash
grpcurl -plaintext -d '{
  "key": "user:123",
  "consistency_w": 2,
  "client_id": "client1",
  "request_id": "req3"
}' localhost:50051 kvstore.KVStore/Delete
```

### Debug Endpoints

**Get Membership:**
```bash
grpcurl -plaintext -d '{}' localhost:50051 kvstore.Membership/GetMembership
```

**Get Ring Info:**
```bash
grpcurl -plaintext -d '{"key": "user:123"}' localhost:50051 kvstore.Membership/GetRing
```

**Health Check:**
```bash
grpcurl -plaintext -d '{}' localhost:50051 kvstore.Membership/Health
```

## Consistency Model

### Quorum Parameters

- **N (Replication Factor)**: Number of replicas per key (default: 3)
- **R (Read Quorum)**: Number of successful reads required (default: 2)
- **W (Write Quorum)**: Number of successful writes required (default: 2)

### Consistency Guarantees

- **R + W > N**: Strong consistency (read-after-write)
- **R + W ≤ N**: Eventual consistency (may read stale data)
- **Default (R=2, W=2, N=3)**: Eventual consistency with high availability

### Conflict Semantics

- **Dominance**: If version A dominates B, A happened after B
- **Concurrency**: If versions are concurrent, operations happened independently
- **Resolution**: Client receives siblings and resolves conflicts
- **Write Context**: Client can provide version context to ensure new write dominates siblings

## Limitations

This is a learning-grade implementation with the following limitations:

1. **No Background Anti-Entropy**: No Merkle trees or periodic scans
2. **Simplified Membership**: SWIM-style but not production-hardened
3. **No Rebalancing**: Data is not migrated when nodes join/leave
4. **In-Memory Only**: Data is lost on restart (no persistence)
5. **No Hinted Handoff**: Writes fail if replica is down
6. **Static Configuration**: No dynamic configuration changes
7. **No Authentication**: No security/authorization

## Roadmap

- [ ] Persistence (write-ahead log, snapshots)
- [ ] Background anti-entropy (Merkle trees)
- [ ] Hinted handoff
- [ ] Dynamic configuration
- [ ] Metrics and observability
- [ ] Client libraries (Go, Python, etc.)

## Development

### Project Structure

```
kvstore/
├── api/                    # Protobuf definitions
├── cmd/kvstore/           # CLI entrypoint
├── internal/
│   ├── clock/             # Vector clocks
│   ├── config/            # Configuration parsing
│   ├── gossip/            # Membership protocol
│   ├── node/              # Node runtime
│   ├── quorum/            # Quorum coordination
│   ├── repair/            # Conflict reconciliation & read repair
│   ├── replication/       # Replica selection
│   ├── ring/              # Consistent hashing
│   └── storage/           # Key-value storage
├── scripts/               # Cluster management & demos
└── Makefile               # Build targets
```

### Running Tests

```bash
# All tests (unit + integration)
make test

# Unit tests only (fast)
make test-short

# Integration tests (smoke tests)
make smoke

# Specific package
go test ./internal/repair -v

# With coverage
go test ./... -cover

# Property-based tests
go test ./internal/ring -run Property
go test ./internal/clock -run Property
go test ./internal/quorum -run Property
```

**Test Structure:**
- **Unit Tests**: Fast, isolated tests in each package (`*_test.go`)
- **Property Tests**: Test invariants and properties (`*_property_test.go`)
- **Integration Tests**: End-to-end cluster tests (`internal/it/smoke_test.go`)

**Debugging Test Failures:**
- Integration test logs: `.local/it-logs/n*.log`
- Use `-v` flag for verbose output: `go test ./internal/it/... -v`
- Check cluster status: `./scripts/cluster.sh status`

### Logs

Cluster logs are stored in `.local/logs/`:
- `n1.log`, `n2.log`, `n3.log` - Node logs
- Use `./scripts/cluster.sh logs n1` to tail logs

### Cluster Management

```bash
# Start cluster
./scripts/cluster.sh up [num_nodes] [rf] [r] [w] [vnodes]

# Stop cluster
./scripts/cluster.sh down

# Check status
./scripts/cluster.sh status

# Kill specific node
./scripts/cluster.sh kill n2

# Restart node
./scripts/cluster.sh restart n2

# View logs
./scripts/cluster.sh logs n1 [lines]
```

## License

MIT
