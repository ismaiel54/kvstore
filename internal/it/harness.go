package it

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	kvstorepb "kvstore/internal/gen/api"
)

// Cluster represents a test cluster of nodes
type Cluster struct {
	nodes      []*Node
	logDir     string
	binaryPath string
	mu         sync.Mutex
}

// Node represents a single node in the test cluster
type Node struct {
	ID           string
	Addr         string
	Port         int
	cmd          *exec.Cmd
	logFile      *os.File
	client       kvstorepb.KVStoreClient
	healthClient kvstorepb.MembershipClient
}

// NewCluster creates a new test cluster harness
func NewCluster(binaryPath string) (*Cluster, error) {
	logDir := filepath.Join(".local", "it-logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	return &Cluster{
		nodes:      make([]*Node, 0),
		logDir:     logDir,
		binaryPath: binaryPath,
	}, nil
}

// StartNode starts a single node in the cluster
func (c *Cluster) StartNode(ctx context.Context, nodeID string, port int, seeds []string, rf, r, w int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Build peer list (all existing nodes + self)
	peers := make([]string, 0)
	for _, n := range c.nodes {
		peers = append(peers, fmt.Sprintf("%s=127.0.0.1:%d", n.ID, n.Port))
	}

	// Add seed nodes if provided
	if len(seeds) > 0 {
		peers = append(peers, seeds...)
	}

	peerStr := ""
	for i, p := range peers {
		if i > 0 {
			peerStr += ","
		}
		peerStr += p
	}

	addr := fmt.Sprintf(":%d", port)
	logPath := filepath.Join(c.logDir, fmt.Sprintf("%s.log", nodeID))
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	cmd := exec.CommandContext(ctx, c.binaryPath,
		"--node-id", nodeID,
		"--listen", addr,
		"--peers", peerStr,
		"--rf", fmt.Sprintf("%d", rf),
		"--r", fmt.Sprintf("%d", r),
		"--w", fmt.Sprintf("%d", w),
		"--vnodes", "128",
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start node %s: %w", nodeID, err)
	}

	// Connect gRPC client
	conn, err := grpc.Dial(
		fmt.Sprintf("127.0.0.1:%d", port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)
	if err != nil {
		cmd.Process.Kill()
		logFile.Close()
		return fmt.Errorf("failed to dial node %s: %w", nodeID, err)
	}

	node := &Node{
		ID:           nodeID,
		Addr:         fmt.Sprintf("127.0.0.1:%d", port),
		Port:         port,
		cmd:          cmd,
		logFile:      logFile,
		client:       kvstorepb.NewKVStoreClient(conn),
		healthClient: kvstorepb.NewMembershipClient(conn),
	}

	c.nodes = append(c.nodes, node)

	// Wait for node to be ready
	if err := c.waitForReady(ctx, node, 10*time.Second); err != nil {
		node.Stop()
		return fmt.Errorf("node %s failed to become ready: %w", nodeID, err)
	}

	return nil
}

// waitForReady waits for a node to be ready by checking health endpoint
func (c *Cluster) waitForReady(ctx context.Context, node *Node, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for node %s to be ready", node.ID)
			}

			healthCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			_, err := node.healthClient.Health(healthCtx, &kvstorepb.HealthRequest{})
			cancel()

			if err == nil {
				return nil
			}
		}
	}
}

// Stop stops all nodes in the cluster
func (c *Cluster) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, node := range c.nodes {
		node.Stop()
	}
	c.nodes = nil
}

// Stop stops a single node
func (n *Node) Stop() {
	if n.cmd != nil && n.cmd.Process != nil {
		n.cmd.Process.Kill()
		n.cmd.Wait()
	}
	if n.logFile != nil {
		n.logFile.Close()
	}
}

// GetClient returns the KVStore client for a node
func (n *Node) GetClient() kvstorepb.KVStoreClient {
	return n.client
}

// GetHealthClient returns the Membership client for a node
func (n *Node) GetHealthClient() kvstorepb.MembershipClient {
	return n.healthClient
}

// StartCluster starts a 3-node cluster with default settings
func (c *Cluster) StartCluster(ctx context.Context) error {
	// Build binary if needed
	if c.binaryPath == "" {
		c.binaryPath = "./kvstore"
	}
	if _, err := os.Stat(c.binaryPath); os.IsNotExist(err) {
		return fmt.Errorf("binary not found at %s, build it first with 'go build -o kvstore ./cmd/kvstore'", c.binaryPath)
	}

	// Start nodes sequentially
	basePort := 60051
	seeds := []string{} // Will be populated as nodes start

	for i := 1; i <= 3; i++ {
		nodeID := fmt.Sprintf("n%d", i)
		port := basePort + i - 1

		// First node has no seeds, others use n1 as seed
		if i == 1 {
			seeds = []string{}
		} else {
			seeds = []string{fmt.Sprintf("n1=127.0.0.1:%d", basePort)}
		}

		if err := c.StartNode(ctx, nodeID, port, seeds, 3, 2, 2); err != nil {
			c.Stop()
			return fmt.Errorf("failed to start node %s: %w", nodeID, err)
		}

		// Wait a bit between starts
		time.Sleep(1 * time.Second)
	}

	return nil
}

// GetNode returns a node by ID
func (c *Cluster) GetNode(nodeID string) *Node {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, n := range c.nodes {
		if n.ID == nodeID {
			return n
		}
	}
	return nil
}

// KillNode kills a specific node
func (c *Cluster) KillNode(nodeID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, node := range c.nodes {
		if node.ID == nodeID {
			if node.cmd != nil && node.cmd.Process != nil {
				if err := node.cmd.Process.Kill(); err != nil {
					return fmt.Errorf("failed to kill node %s: %w", nodeID, err)
				}
				node.cmd.Wait()
			}
			return nil
		}
	}
	return fmt.Errorf("node %s not found", nodeID)
}

// RestartNode restarts a specific node
func (c *Cluster) RestartNode(ctx context.Context, nodeID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var node *Node
	var index int
	for i, n := range c.nodes {
		if n.ID == nodeID {
			node = n
			index = i
			break
		}
	}
	if node == nil {
		return fmt.Errorf("node %s not found", nodeID)
	}

	// Stop the node
	if node.cmd != nil && node.cmd.Process != nil {
		node.cmd.Process.Kill()
		node.cmd.Wait()
	}
	if node.logFile != nil {
		node.logFile.Close()
	}

	// Recreate log file
	logPath := filepath.Join(c.logDir, fmt.Sprintf("%s.log", nodeID))
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to recreate log file: %w", err)
	}

	// Build peer list
	peers := make([]string, 0)
	for _, n := range c.nodes {
		if n.ID != nodeID {
			peers = append(peers, fmt.Sprintf("%s=127.0.0.1:%d", n.ID, n.Port))
		}
	}
	if len(peers) == 0 {
		peers = []string{}
	} else {
		peers = []string{fmt.Sprintf("n1=127.0.0.1:%d", 60051)}
	}

	peerStr := ""
	for i, p := range peers {
		if i > 0 {
			peerStr += ","
		}
		peerStr += p
	}

	// Restart
	cmd := exec.CommandContext(ctx, c.binaryPath,
		"--node-id", nodeID,
		"--listen", fmt.Sprintf(":%d", node.Port),
		"--peers", peerStr,
		"--rf", "3",
		"--r", "2",
		"--w", "2",
		"--vnodes", "128",
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to restart node %s: %w", nodeID, err)
	}

	// Update node
	node.cmd = cmd
	node.logFile = logFile

	// Reconnect client
	conn, err := grpc.Dial(
		node.Addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)
	if err != nil {
		return fmt.Errorf("failed to reconnect to node %s: %w", nodeID, err)
	}

	node.client = kvstorepb.NewKVStoreClient(conn)
	node.healthClient = kvstorepb.NewMembershipClient(conn)

	// Wait for ready
	if err := c.waitForReady(ctx, node, 10*time.Second); err != nil {
		return fmt.Errorf("node %s failed to become ready after restart: %w", nodeID, err)
	}

	c.nodes[index] = node
	return nil
}
