#!/bin/bash

# Demo: Quorum tolerance
# Shows that system continues operating with 1 node down (R=2, W=2)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}=== Quorum Tolerance Demo ===${NC}"
echo ""

# Start cluster
echo "1. Starting 3-node cluster..."
./scripts/cluster.sh up 3
sleep 3

# Put initial value
echo ""
echo "2. Putting key 'quorum-test' with value 'initial'..."
grpcurl -plaintext -d '{
  "key": "quorum-test",
  "value": "aW5pdGlhbA==",
  "consistency_w": 2,
  "client_id": "demo",
  "request_id": "req1"
}' localhost:50051 kvstore.KVStore/Put > /dev/null
echo "[OK] Put successful"

# Get value
echo ""
echo "3. Getting key 'quorum-test'..."
RESPONSE=$(grpcurl -plaintext -d '{
  "key": "quorum-test",
  "consistency_r": 2,
  "client_id": "demo",
  "request_id": "req2"
}' localhost:50051 kvstore.KVStore/Get)
echo "[OK] Get successful"
echo "Response: $RESPONSE" | head -5

# Kill one node
echo ""
echo "4. Killing node n3..."
./scripts/cluster.sh kill n3
sleep 2

# Put should still work (W=2, 2 nodes available)
echo ""
echo "5. Putting new value 'updated' (should still work with W=2)..."
grpcurl -plaintext -d '{
  "key": "quorum-test",
  "value": "dXBkYXRlZA==",
  "consistency_w": 2,
  "client_id": "demo",
  "request_id": "req3"
}' localhost:50051 kvstore.KVStore/Put > /dev/null
echo "[OK] Put successful (quorum met with 2 nodes)"

# Get should still work (R=2, 2 nodes available)
echo ""
echo "6. Getting key again (should still work with R=2)..."
RESPONSE=$(grpcurl -plaintext -d '{
  "key": "quorum-test",
  "consistency_r": 2,
  "client_id": "demo",
  "request_id": "req4"
}' localhost:50051 kvstore.KVStore/Get)
echo "[OK] Get successful (quorum met with 2 nodes)"
echo "Response: $RESPONSE" | head -5

# Cleanup
echo ""
echo "7. Stopping cluster..."
./scripts/cluster.sh down

echo ""
echo -e "${GREEN}=== Demo Complete ===${NC}"
echo "The system continued operating with 1 node down because R=2, W=2 and 2 nodes remained available."

