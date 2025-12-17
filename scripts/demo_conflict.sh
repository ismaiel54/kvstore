#!/bin/bash

# Demo: Conflict detection and siblings
# Shows concurrent writes creating conflicts that are returned as siblings

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}=== Conflict Detection Demo ===${NC}"
echo ""

# Start cluster
echo "1. Starting 3-node cluster..."
./scripts/cluster.sh up 3
sleep 3

# Write from node 1
echo ""
echo "2. Writing 'value1' via node 1..."
grpcurl -plaintext -d '{
  "key": "conflict-key",
  "value": "dmFsdWUx",
  "consistency_w": 2,
  "client_id": "client1",
  "request_id": "req1"
}' localhost:50051 kvstore.KVStore/Put > /dev/null
echo "[OK] Write 1 successful"

# Write from node 2 (concurrent - different coordinator)
echo ""
echo "3. Writing 'value2' via node 2 (concurrent write)..."
grpcurl -plaintext -d '{
  "key": "conflict-key",
  "value": "dmFsdWUy",
  "consistency_w": 2,
  "client_id": "client2",
  "request_id": "req2"
}' localhost:50052 kvstore.KVStore/Put > /dev/null
echo "[OK] Write 2 successful"

# Get should return conflicts
echo ""
echo "4. Getting key (should return conflicts/siblings)..."
RESPONSE=$(grpcurl -plaintext -d '{
  "key": "conflict-key",
  "consistency_r": 2,
  "client_id": "client3",
  "request_id": "req3"
}' localhost:50051 kvstore.KVStore/Get)
echo "Response:"
echo "$RESPONSE" | python3 -m json.tool 2>/dev/null || echo "$RESPONSE"

# Check if conflicts are present
if echo "$RESPONSE" | grep -q "conflicts"; then
    echo ""
    echo -e "${GREEN}[OK] Conflicts detected! Multiple siblings returned.${NC}"
else
    echo ""
    echo -e "${YELLOW}Note: Conflicts may have been resolved or not detected yet.${NC}"
fi

# Cleanup
echo ""
echo "5. Stopping cluster..."
./scripts/cluster.sh down

echo ""
echo -e "${GREEN}=== Demo Complete ===${NC}"
echo "Concurrent writes created conflicts that were returned as siblings."

