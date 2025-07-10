package relay

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/network"
)

// ConnectionPool manages persistent connections to relay nodes
type ConnectionPool struct {
	connections map[peer.ID]*RelayConnection
	config      *ConnectionPoolConfig
	metrics     *ConnectionPoolMetrics
	mu          sync.RWMutex
}

// ConnectionPoolConfig contains configuration for the connection pool
type ConnectionPoolConfig struct {
	MaxConnections      int           // Maximum number of connections to maintain
	MaxIdleTime         time.Duration // Maximum time a connection can be idle
	ConnectionTimeout   time.Duration // Timeout for establishing connections
	KeepAliveInterval   time.Duration // Interval for keep-alive messages
	ReconnectAttempts   int           // Number of reconnection attempts
	ReconnectDelay      time.Duration // Delay between reconnection attempts
	MaxRequestsPerConn  int           // Maximum concurrent requests per connection
}

// RelayConnection represents a persistent connection to a relay node
type RelayConnection struct {
	PeerID        peer.ID
	Stream        network.Stream
	Protocol      *RelayProtocol
	Status        ConnectionStatus
	CreatedAt     time.Time
	LastUsed      time.Time
	RequestCount  int
	ActiveRequests int
	mu            sync.RWMutex
}

// ConnectionStatus represents the status of a connection
type ConnectionStatus int

const (
	ConnectionStatusDisconnected ConnectionStatus = iota
	ConnectionStatusConnecting
	ConnectionStatusConnected
	ConnectionStatusReconnecting
	ConnectionStatusClosed
)

// ConnectionPoolMetrics tracks connection pool performance
type ConnectionPoolMetrics struct {
	TotalConnections    int
	ActiveConnections   int
	IdleConnections     int
	FailedConnections   int
	TotalRequests       int64
	ConnectionReuses    int64
	AverageConnTime     time.Duration
	LastUpdate          time.Time
}

// NewConnectionPool creates a new connection pool
func NewConnectionPool(config *ConnectionPoolConfig) *ConnectionPool {
	return &ConnectionPool{
		connections: make(map[peer.ID]*RelayConnection),
		config:      config,
		metrics:     &ConnectionPoolMetrics{},
	}
}

// GetConnection gets or creates a connection to a relay node
func (cp *ConnectionPool) GetConnection(ctx context.Context, peerID peer.ID) (*RelayConnection, error) {
	cp.mu.RLock()
	conn, exists := cp.connections[peerID]
	cp.mu.RUnlock()
	
	if exists && conn.Status == ConnectionStatusConnected {
		// Reuse existing connection
		conn.mu.Lock()
		conn.LastUsed = time.Now()
		conn.mu.Unlock()
		
		cp.mu.Lock()
		cp.metrics.ConnectionReuses++
		cp.mu.Unlock()
		
		return conn, nil
	}
	
	// Create new connection
	return cp.createConnection(ctx, peerID)
}

// createConnection creates a new connection to a relay node
func (cp *ConnectionPool) createConnection(ctx context.Context, peerID peer.ID) (*RelayConnection, error) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	
	// Check if we've reached the maximum number of connections
	if len(cp.connections) >= cp.config.MaxConnections {
		// Remove an idle connection to make room
		cp.removeIdleConnection()
	}
	
	// Create new connection
	conn := &RelayConnection{
		PeerID:    peerID,
		Status:    ConnectionStatusConnecting,
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
	}
	
	// Initialize protocol
	protocol, err := NewRelayProtocol()
	if err != nil {
		return nil, fmt.Errorf("failed to create relay protocol: %w", err)
	}
	conn.Protocol = protocol
	
	// Store connection
	cp.connections[peerID] = conn
	
	// Establish connection in background
	go cp.establishConnection(ctx, conn)
	
	return conn, nil
}

// establishConnection establishes a connection to a relay node
func (cp *ConnectionPool) establishConnection(ctx context.Context, conn *RelayConnection) {
	// Add timeout to context
	ctx, cancel := context.WithTimeout(ctx, cp.config.ConnectionTimeout)
	defer cancel()
	
	// Simulate connection establishment
	// In a real implementation, this would:
	// 1. Dial the peer using libp2p
	// 2. Open a stream with the relay protocol
	// 3. Perform key exchange
	// 4. Send initial handshake
	
	start := time.Now()
	
	// Simulate connection delay
	select {
	case <-time.After(100 * time.Millisecond):
		// Connection successful
		conn.mu.Lock()
		conn.Status = ConnectionStatusConnected
		conn.mu.Unlock()
		
		cp.mu.Lock()
		cp.metrics.ActiveConnections++
		cp.metrics.AverageConnTime = time.Since(start)
		cp.mu.Unlock()
		
		// Start keep-alive routine
		go cp.keepAlive(conn)
		
	case <-ctx.Done():
		// Connection failed
		conn.mu.Lock()
		conn.Status = ConnectionStatusDisconnected
		conn.mu.Unlock()
		
		cp.mu.Lock()
		cp.metrics.FailedConnections++
		cp.mu.Unlock()
	}
}

// SendRequest sends a request through a connection
func (cp *ConnectionPool) SendRequest(ctx context.Context, conn *RelayConnection, msg *RelayMessage) (*RelayMessage, error) {
	if conn.Status != ConnectionStatusConnected {
		return nil, fmt.Errorf("connection not ready")
	}
	
	// Check if connection is overloaded
	conn.mu.RLock()
	if conn.ActiveRequests >= cp.config.MaxRequestsPerConn {
		conn.mu.RUnlock()
		return nil, fmt.Errorf("connection overloaded")
	}
	conn.mu.RUnlock()
	
	// Increment active requests
	conn.mu.Lock()
	conn.ActiveRequests++
	conn.RequestCount++
	conn.LastUsed = time.Now()
	conn.mu.Unlock()
	
	// Decrement when done
	defer func() {
		conn.mu.Lock()
		conn.ActiveRequests--
		conn.mu.Unlock()
	}()
	
	// Simulate sending request and receiving response
	// In a real implementation, this would:
	// 1. Serialize the message
	// 2. Send it through the stream
	// 3. Wait for response
	// 4. Deserialize the response
	
	select {
	case <-time.After(50 * time.Millisecond):
		// Simulate successful response
		response := &RelayMessage{
			Type:      MsgTypeBlockResponse,
			ID:        msg.ID + "_response",
			Timestamp: time.Now(),
		}
		
		cp.mu.Lock()
		cp.metrics.TotalRequests++
		cp.mu.Unlock()
		
		return response, nil
		
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// CloseConnection closes a connection to a relay node
func (cp *ConnectionPool) CloseConnection(peerID peer.ID) error {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	
	conn, exists := cp.connections[peerID]
	if !exists {
		return fmt.Errorf("connection not found")
	}
	
	// Close the connection
	conn.mu.Lock()
	conn.Status = ConnectionStatusClosed
	if conn.Stream != nil {
		conn.Stream.Close()
	}
	conn.mu.Unlock()
	
	// Remove from pool
	delete(cp.connections, peerID)
	
	// Update metrics
	cp.metrics.ActiveConnections--
	
	return nil
}

// keepAlive sends keep-alive messages to maintain the connection
func (cp *ConnectionPool) keepAlive(conn *RelayConnection) {
	ticker := time.NewTicker(cp.config.KeepAliveInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// Check if connection is still valid
			conn.mu.RLock()
			if conn.Status != ConnectionStatusConnected {
				conn.mu.RUnlock()
				return
			}
			
			// Check if connection has been idle too long
			if time.Since(conn.LastUsed) > cp.config.MaxIdleTime {
				conn.mu.RUnlock()
				cp.CloseConnection(conn.PeerID)
				return
			}
			conn.mu.RUnlock()
			
			// Send keep-alive (health check)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			healthCheck, err := conn.Protocol.CreateHealthCheck(ctx, "", conn.PeerID)
			cancel()
			
			if err != nil {
				continue
			}
			
			// Send the health check
			_, err = cp.SendRequest(context.Background(), conn, healthCheck)
			if err != nil {
				// Connection failed, try to reconnect
				go cp.reconnectConnection(conn)
				return
			}
		}
	}
}

// reconnectConnection attempts to reconnect a failed connection
func (cp *ConnectionPool) reconnectConnection(conn *RelayConnection) {
	conn.mu.Lock()
	if conn.Status == ConnectionStatusReconnecting {
		conn.mu.Unlock()
		return // Already reconnecting
	}
	conn.Status = ConnectionStatusReconnecting
	conn.mu.Unlock()
	
	for attempt := 0; attempt < cp.config.ReconnectAttempts; attempt++ {
		// Wait before retrying
		time.Sleep(cp.config.ReconnectDelay)
		
		// Try to establish connection
		ctx, cancel := context.WithTimeout(context.Background(), cp.config.ConnectionTimeout)
		cp.establishConnection(ctx, conn)
		cancel()
		
		// Check if reconnection succeeded
		conn.mu.RLock()
		if conn.Status == ConnectionStatusConnected {
			conn.mu.RUnlock()
			return
		}
		conn.mu.RUnlock()
	}
	
	// Reconnection failed, close the connection
	cp.CloseConnection(conn.PeerID)
}

// removeIdleConnection removes an idle connection to make room for new ones
func (cp *ConnectionPool) removeIdleConnection() {
	var oldestConn *RelayConnection
	var oldestPeerID peer.ID
	
	for peerID, conn := range cp.connections {
		if conn.ActiveRequests == 0 { // Only consider idle connections
			if oldestConn == nil || conn.LastUsed.Before(oldestConn.LastUsed) {
				oldestConn = conn
				oldestPeerID = peerID
			}
		}
	}
	
	if oldestConn != nil {
		cp.CloseConnection(oldestPeerID)
	}
}

// GetMetrics returns current connection pool metrics
func (cp *ConnectionPool) GetMetrics() *ConnectionPoolMetrics {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	
	// Update current metrics
	cp.metrics.TotalConnections = len(cp.connections)
	cp.metrics.ActiveConnections = 0
	cp.metrics.IdleConnections = 0
	
	for _, conn := range cp.connections {
		if conn.Status == ConnectionStatusConnected {
			if conn.ActiveRequests > 0 {
				cp.metrics.ActiveConnections++
			} else {
				cp.metrics.IdleConnections++
			}
		}
	}
	
	cp.metrics.LastUpdate = time.Now()
	
	return cp.metrics
}

// Close closes all connections in the pool
func (cp *ConnectionPool) Close() error {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	
	for peerID := range cp.connections {
		cp.CloseConnection(peerID)
	}
	
	return nil
}

// GetConnectionInfo returns information about a specific connection
func (cp *ConnectionPool) GetConnectionInfo(peerID peer.ID) (*ConnectionInfo, error) {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	
	conn, exists := cp.connections[peerID]
	if !exists {
		return nil, fmt.Errorf("connection not found")
	}
	
	conn.mu.RLock()
	defer conn.mu.RUnlock()
	
	return &ConnectionInfo{
		PeerID:         conn.PeerID,
		Status:         conn.Status,
		CreatedAt:      conn.CreatedAt,
		LastUsed:       conn.LastUsed,
		RequestCount:   conn.RequestCount,
		ActiveRequests: conn.ActiveRequests,
	}, nil
}

// ConnectionInfo contains information about a connection
type ConnectionInfo struct {
	PeerID         peer.ID
	Status         ConnectionStatus
	CreatedAt      time.Time
	LastUsed       time.Time
	RequestCount   int
	ActiveRequests int
}