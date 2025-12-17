#!/bin/bash
# Run a 3-node local cluster for testing

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Cleanup function
cleanup() {
    echo -e "\n${YELLOW}Stopping all nodes...${NC}"
    pkill -f "go run ./cmd/kvstore" || true
    pkill -f "kvstore" || true
    wait
    echo -e "${GREEN}All nodes stopped.${NC}"
}

# Trap SIGINT and SIGTERM
trap cleanup SIGINT SIGTERM

# Build the binary first
echo -e "${GREEN}Building kvstore...${NC}"
cd "$(dirname "$0")/.."
go build -o /tmp/kvstore ./cmd/kvstore

# Define peer list (all nodes know about all other nodes)
PEERS="n1=127.0.0.1:50051,n2=127.0.0.1:50052,n3=127.0.0.1:50053"

# Replication settings (can be overridden via environment)
RF=${RF:-3}
R=${R:-2}
W=${W:-2}

echo -e "${GREEN}Replication settings: RF=$RF, R=$R, W=$W${NC}"

# Start node 1
echo -e "${GREEN}Starting node 1 on :50051...${NC}"
/tmp/kvstore --node-id=n1 --listen=:50051 --peers="$PEERS" --rf=$RF --r=$R --w=$W > /tmp/kvstore-n1.log 2>&1 &
NODE1_PID=$!
sleep 1

# Start node 2
echo -e "${GREEN}Starting node 2 on :50052...${NC}"
/tmp/kvstore --node-id=n2 --listen=:50052 --peers="$PEERS" --rf=$RF --r=$R --w=$W > /tmp/kvstore-n2.log 2>&1 &
NODE2_PID=$!
sleep 1

# Start node 3
echo -e "${GREEN}Starting node 3 on :50053...${NC}"
/tmp/kvstore --node-id=n3 --listen=:50053 --peers="$PEERS" --rf=$RF --r=$R --w=$W > /tmp/kvstore-n3.log 2>&1 &
NODE3_PID=$!
sleep 1

echo -e "${GREEN}All nodes started!${NC}"
echo -e "${YELLOW}Node 1: PID $NODE1_PID, logs: /tmp/kvstore-n1.log${NC}"
echo -e "${YELLOW}Node 2: PID $NODE2_PID, logs: /tmp/kvstore-n2.log${NC}"
echo -e "${YELLOW}Node 3: PID $NODE3_PID, logs: /tmp/kvstore-n3.log${NC}"
echo -e "${YELLOW}Press Ctrl+C to stop all nodes${NC}"

# Wait for all background processes
wait

