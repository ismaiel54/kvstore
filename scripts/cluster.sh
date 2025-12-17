#!/bin/bash

# Cluster management script for kvstore
# Usage: ./scripts/cluster.sh [command] [args]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
LOCAL_DIR="$PROJECT_ROOT/.local"
LOGS_DIR="$LOCAL_DIR/logs"
PIDS_DIR="$LOCAL_DIR/pids"
BINARY="$PROJECT_ROOT/kvstore"

# Build binary if it doesn't exist
if [ ! -f "$BINARY" ]; then
    echo -e "${YELLOW}Building binary...${NC}"
    cd "$PROJECT_ROOT"
    go build -o "$BINARY" ./cmd/kvstore
    if [ $? -ne 0 ]; then
        echo -e "${RED}Failed to build binary${NC}"
        exit 1
    fi
fi

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Default config
DEFAULT_RF=3
DEFAULT_R=2
DEFAULT_W=2
DEFAULT_VNODES=128
BASE_PORT=50051

# Ensure directories exist
mkdir -p "$LOGS_DIR" "$PIDS_DIR"

# Cleanup function
cleanup() {
    echo -e "\n${YELLOW}Cleaning up...${NC}"
    ./scripts/cluster.sh down
    exit 0
}

trap cleanup INT TERM

# Get node address by ID
get_node_addr() {
    local node_id=$1
    local port=$((BASE_PORT + ${node_id#n} - 1))
    echo "127.0.0.1:$port"
}

# Get node port by ID
get_node_port() {
    local node_id=$1
    echo $((BASE_PORT + ${node_id#n} - 1))
}

# Start a single node
start_node() {
    local node_id=$1
    local num_nodes=$2
    local rf=${3:-$DEFAULT_RF}
    local r=${4:-$DEFAULT_R}
    local w=${5:-$DEFAULT_W}
    local vnodes=${6:-$DEFAULT_VNODES}
    
    local port=$(get_node_port "$node_id")
    local addr=$(get_node_addr "$node_id")
    local log_file="$LOGS_DIR/${node_id}.log"
    local pid_file="$PIDS_DIR/${node_id}.pid"
    
    # Build seed list (n1 is always a seed)
    local seeds="n1=$(get_node_addr n1)"
    if [ "$node_id" != "n1" ]; then
        # Other nodes use n1 as seed
        seeds="n1=$(get_node_addr n1)"
    fi
    
    # Build peer list (all nodes know about all others)
    local peers=""
    for i in $(seq 1 $num_nodes); do
        if [ $i -ne ${node_id#n} ]; then
            local peer_id="n$i"
            local peer_addr=$(get_node_addr "$peer_id")
            if [ -z "$peers" ]; then
                peers="$peer_id=$peer_addr"
            else
                peers="$peers,$peer_id=$peer_addr"
            fi
        fi
    done
    
    # Check if already running
    if [ -f "$pid_file" ]; then
        local pid=$(cat "$pid_file")
        if ps -p "$pid" > /dev/null 2>&1; then
            echo -e "${YELLOW}Node $node_id already running (PID: $pid)${NC}"
            return 0
        fi
    fi
    
    echo -e "${GREEN}Starting $node_id on :$port...${NC}"
    
    # Build and run
    cd "$PROJECT_ROOT"
    if [ ! -f "$BINARY" ]; then
        echo -e "${YELLOW}Building binary...${NC}"
        go build -o "$BINARY" ./cmd/kvstore
    fi
    
    "$BINARY" \
        --node-id="$node_id" \
        --listen=":$port" \
        --peers="$peers" \
        --rf="$rf" \
        --r="$r" \
        --w="$w" \
        --vnodes="$vnodes" \
        > "$log_file" 2>&1 &
    
    local pid=$!
    echo $pid > "$pid_file"
    echo -e "${GREEN}$node_id started (PID: $pid, port: $port)${NC}"
    
    sleep 1
}

# Stop a single node
stop_node() {
    local node_id=$1
    local pid_file="$PIDS_DIR/${node_id}.pid"
    
    if [ ! -f "$pid_file" ]; then
        echo -e "${YELLOW}Node $node_id not running${NC}"
        return 0
    fi
    
    local pid=$(cat "$pid_file")
    if ! ps -p "$pid" > /dev/null 2>&1; then
        echo -e "${YELLOW}Node $node_id not running (stale PID file)${NC}"
        rm -f "$pid_file"
        return 0
    fi
    
    echo -e "${YELLOW}Stopping $node_id (PID: $pid)...${NC}"
    kill "$pid" 2>/dev/null || true
    sleep 1
    
    if ps -p "$pid" > /dev/null 2>&1; then
        kill -9 "$pid" 2>/dev/null || true
    fi
    
    rm -f "$pid_file"
    echo -e "${GREEN}$node_id stopped${NC}"
}

# Command: up
cmd_up() {
    local num_nodes=${1:-3}
    local rf=${2:-$DEFAULT_RF}
    local r=${3:-$DEFAULT_R}
    local w=${4:-$DEFAULT_W}
    local vnodes=${5:-$DEFAULT_VNODES}
    
    if [ "$num_nodes" -lt 1 ] || [ "$num_nodes" -gt 10 ]; then
        echo -e "${RED}Error: num_nodes must be between 1 and 10${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}Starting $num_nodes-node cluster (rf=$rf, r=$r, w=$w, vnodes=$vnodes)...${NC}"
    
    # Start nodes sequentially
    for i in $(seq 1 $num_nodes); do
        start_node "n$i" "$num_nodes" "$rf" "$r" "$w" "$vnodes"
    done
    
    echo -e "${GREEN}Cluster started!${NC}"
    echo -e "Logs: $LOGS_DIR/"
    echo -e "PIDs: $PIDS_DIR/"
    echo -e "Use './scripts/cluster.sh status' to check status"
}

# Command: down
cmd_down() {
    echo -e "${YELLOW}Stopping all nodes...${NC}"
    
    if [ ! -d "$PIDS_DIR" ]; then
        echo -e "${YELLOW}No nodes running${NC}"
        return 0
    fi
    
    for pid_file in "$PIDS_DIR"/*.pid; do
        if [ -f "$pid_file" ]; then
            local node_id=$(basename "$pid_file" .pid)
            stop_node "$node_id"
        fi
    done
    
    echo -e "${GREEN}All nodes stopped${NC}"
}

# Command: status
cmd_status() {
    echo -e "${GREEN}Cluster Status:${NC}"
    echo ""
    
    if [ ! -d "$PIDS_DIR" ]; then
        echo -e "${YELLOW}No nodes running${NC}"
        return 0
    fi
    
    local running=0
    local stopped=0
    
    for pid_file in "$PIDS_DIR"/*.pid; do
        if [ -f "$pid_file" ]; then
            local node_id=$(basename "$pid_file" .pid)
            local pid=$(cat "$pid_file")
            local port=$(get_node_port "$node_id")
            
            if ps -p "$pid" > /dev/null 2>&1; then
                echo -e "${GREEN}[RUNNING] $node_id: PID $pid, port :$port${NC}"
                running=$((running + 1))
            else
                echo -e "${RED}[STOPPED] $node_id: not running (stale PID: $pid)${NC}"
                stopped=$((stopped + 1))
            fi
        fi
    done
    
    echo ""
    echo "Running: $running, Stopped: $stopped"
}

# Command: kill
cmd_kill() {
    local node_id=$1
    
    if [ -z "$node_id" ]; then
        echo -e "${RED}Error: node_id required (e.g., n1, n2)${NC}"
        exit 1
    fi
    
    stop_node "$node_id"
}

# Command: restart
cmd_restart() {
    local node_id=$1
    local num_nodes=${2:-3}
    local rf=${3:-$DEFAULT_RF}
    local r=${4:-$DEFAULT_R}
    local w=${5:-$DEFAULT_W}
    local vnodes=${6:-$DEFAULT_VNODES}
    
    if [ -z "$node_id" ]; then
        echo -e "${RED}Error: node_id required (e.g., n1, n2)${NC}"
        exit 1
    fi
    
    echo -e "${YELLOW}Restarting $node_id...${NC}"
    stop_node "$node_id"
    sleep 1
    start_node "$node_id" "$num_nodes" "$rf" "$r" "$w" "$vnodes"
}

# Command: logs
cmd_logs() {
    local node_id=$1
    local lines=${2:-50}
    
    if [ -z "$node_id" ]; then
        echo -e "${RED}Error: node_id required (e.g., n1, n2)${NC}"
        exit 1
    fi
    
    local log_file="$LOGS_DIR/${node_id}.log"
    
    if [ ! -f "$log_file" ]; then
        echo -e "${RED}Log file not found: $log_file${NC}"
        exit 1
    fi
    
    tail -n "$lines" -f "$log_file"
}

# Main command dispatcher
case "${1:-}" in
    up)
        cmd_up "${@:2}"
        ;;
    down)
        cmd_down
        ;;
    status)
        cmd_status
        ;;
    kill)
        cmd_kill "$2"
        ;;
    restart)
        cmd_restart "${@:2}"
        ;;
    logs)
        cmd_logs "$2" "${3:-50}"
        ;;
    *)
        echo "Usage: $0 {up|down|status|kill|restart|logs} [args]"
        echo ""
        echo "Commands:"
        echo "  up [num_nodes] [rf] [r] [w] [vnodes]  Start cluster (default: 3 nodes)"
        echo "  down                                    Stop all nodes"
        echo "  status                                  Show cluster status"
        echo "  kill <node_id>                          Kill a specific node"
        echo "  restart <node_id> [num_nodes] [rf] [r] [w] [vnodes]  Restart a node"
        echo "  logs <node_id> [lines]                  Show logs (default: 50 lines)"
        echo ""
        echo "Examples:"
        echo "  $0 up 5              # Start 5-node cluster"
        echo "  $0 kill n3           # Kill node n3"
        echo "  $0 restart n3        # Restart node n3"
        echo "  $0 logs n1           # Show logs for n1"
        exit 1
        ;;
esac

