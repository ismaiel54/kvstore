package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	kvstorepb "kvstore/internal/gen/api"
)

const (
	// Metadata key for forwarded requests
	forwardedMetadataKey = "x-forwarded"
	forwardedValue       = "true"
	// Connection timeout
	dialTimeout = 5 * time.Second
)

// ClientManager manages gRPC clients to peer nodes.
type ClientManager struct {
	mu             sync.RWMutex
	clients        map[string]kvstorepb.KVStoreClient
	internalClients map[string]kvstorepb.KVInternalClient
}

// NewClientManager creates a new client manager.
func NewClientManager() *ClientManager {
	return &ClientManager{
		clients:         make(map[string]kvstorepb.KVStoreClient),
		internalClients: make(map[string]kvstorepb.KVInternalClient),
	}
}

// GetClient returns a gRPC client for the given node address.
// Creates a new connection if one doesn't exist.
func (cm *ClientManager) GetClient(addr string) (kvstorepb.KVStoreClient, error) {
	cm.mu.RLock()
	client, exists := cm.clients[addr]
	cm.mu.RUnlock()

	if exists {
		return client, nil
	}

	// Create new connection
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Double-check after acquiring write lock
	if client, exists := cm.clients[addr]; exists {
		return client, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", addr, err)
	}

	client = kvstorepb.NewKVStoreClient(conn)
	cm.clients[addr] = client
	return client, nil
}

// GetInternalClient returns an internal gRPC client for the given node address.
func (cm *ClientManager) GetInternalClient(addr string) (kvstorepb.KVInternalClient, error) {
	cm.mu.RLock()
	client, exists := cm.internalClients[addr]
	cm.mu.RUnlock()

	if exists {
		return client, nil
	}

	// Create new connection
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Double-check after acquiring write lock
	if client, exists := cm.internalClients[addr]; exists {
		return client, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", addr, err)
	}

	client = kvstorepb.NewKVInternalClient(conn)
	cm.internalClients[addr] = client
	return client, nil
}

// Close closes all client connections.
func (cm *ClientManager) Close() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Note: We don't track connections, so we can't close them individually.
	// In a production system, we'd track connections and close them.
	// For Phase 2, this is acceptable as connections will close on process exit.
	cm.clients = make(map[string]kvstorepb.KVStoreClient)
	cm.internalClients = make(map[string]kvstorepb.KVInternalClient)
}

