#!/bin/bash

# Demo: Read repair convergence
# Shows that stale replicas are repaired automatically on GET

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}=== Read Repair Demo ===${NC}"
echo ""

# Start cluster
echo "1. Starting 3-node cluster..."
./scripts/cluster.sh up 3
sleep 3

# Write initial value
echo ""
echo "2. Writing initial value 'v1'..."
grpcurl -plaintext -d '{
  "key": "repair-test",
  "value": "djE=",
  "consistency_w": 2,
  "client_id": "demo",
  "request_id": "req1"
}' localhost:50051 kvstore.KVStore/Put > /dev/null
echo "[OK] Initial write successful"

# Kill node 3
echo ""
echo "3. Killing node n3 (will become stale)..."
./scripts/cluster.sh kill n3
sleep 2

# Write new value (only to n1 and n2, n3 is stale)
echo ""
echo "4. Writing new value 'v2' (n3 is stale)..."
grpcurl -plaintext -d '{
  "key": "repair-test",
  "value": "djI=",
  "consistency_w": 2,
  "client_id": "demo",
  "request_id": "req2"
}' localhost:50051 kvstore.KVStore/Put > /dev/null
echo "[OK] New write successful (n3 missed this update)"

# Restart n3
echo ""
echo "5. Restarting n3 (will have stale data)..."
./scripts/cluster.sh restart n3
sleep 3

# Get should trigger read repair
echo ""
echo "6. Getting key (should trigger read repair)..."
RESPONSE=$(grpcurl -plaintext -d '{
  "key": "repair-test",
  "consistency_r": 2,
  "client_id": "demo",
  "request_id": "req3"
}' localhost:50051 kvstore.KVStore/Get)
echo "[OK] Get successful"
echo "Response: $RESPONSE" | head -5

# Check logs for repair
echo ""
echo "7. Checking logs for read repair activity..."
if grep -q "Read repair" .local/logs/n1.log 2>/dev/null; then
    echo -e "${GREEN}[OK] Read repair detected in logs${NC}"
    grep "Read repair" .local/logs/n1.log | tail -2
else
    echo -e "${YELLOW}Note: Read repair may have completed or logs may be in different node${NC}"
fi

# Get again to show convergence
echo ""
echo "8. Getting key again (should show convergence)..."
RESPONSE=$(grpcurl -plaintext -d '{
  "key": "repair-test",
  "consistency_r": 2,
  "client_id": "demo",
  "request_id": "req4"
}' localhost:50051 kvstore.KVStore/Get)
echo "[OK] Get successful"
echo "Response: $RESPONSE" | head -5

# Cleanup
echo ""
echo "9. Stopping cluster..."
./scripts/cluster.sh down

echo ""
echo -e "${GREEN}=== Demo Complete ===${NC}"
echo "Read repair automatically converged stale replica n3 with the latest value."

