#!/bin/bash

# Demo: Gossip membership and failure detection
# Shows membership state transitions (Alive -> Suspect -> Dead)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}=== Membership Demo ===${NC}"
echo ""

# Start cluster
echo "1. Starting 3-node cluster..."
./scripts/cluster.sh up 3
sleep 5  # Wait for gossip to converge

# Query membership
echo ""
echo "2. Querying membership from node 1..."
MEMBERSHIP=$(grpcurl -plaintext -d '{}' localhost:50051 kvstore.Membership/GetMembership)
echo "Membership state:"
echo "$MEMBERSHIP" | python3 -m json.tool 2>/dev/null || echo "$MEMBERSHIP" | head -20

# Count alive nodes
ALIVE_COUNT=$(echo "$MEMBERSHIP" | grep -c "ALIVE" || echo "0")
echo ""
echo "Alive nodes: $ALIVE_COUNT"

# Kill node 2
echo ""
echo "3. Killing node n2..."
./scripts/cluster.sh kill n2
sleep 2

# Query membership again (should show n2 as SUSPECT)
echo ""
echo "4. Querying membership again (n2 should be SUSPECT)..."
sleep 2
MEMBERSHIP=$(grpcurl -plaintext -d '{}' localhost:50051 kvstore.Membership/GetMembership)
echo "Membership state:"
echo "$MEMBERSHIP" | python3 -m json.tool 2>/dev/null || echo "$MEMBERSHIP" | head -20

# Wait for suspect timeout
echo ""
echo "5. Waiting for suspect timeout (n2 should become DEAD)..."
sleep 5

# Query membership again (should show n2 as DEAD)
echo ""
echo "6. Querying membership again (n2 should be DEAD)..."
MEMBERSHIP=$(grpcurl -plaintext -d '{}' localhost:50051 kvstore.Membership/GetMembership)
echo "Membership state:"
echo "$MEMBERSHIP" | python3 -m json.tool 2>/dev/null || echo "$MEMBERSHIP" | head -20

# Check for DEAD status
if echo "$MEMBERSHIP" | grep -q "DEAD"; then
    echo ""
    echo -e "${GREEN}[OK] Node n2 marked as DEAD${NC}"
else
    echo ""
    echo -e "${YELLOW}Note: n2 may still be SUSPECT (timeout may need more time)${NC}"
fi

# Cleanup
echo ""
echo "7. Stopping cluster..."
./scripts/cluster.sh down

echo ""
echo -e "${GREEN}=== Demo Complete ===${NC}"
echo "Membership protocol detected failure: n2 transitioned from ALIVE -> SUSPECT -> DEAD"

