package testing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// TestMockIPFSClient validates the MockIPFSClient implementation
func TestMockIPFSClient(t *testing.T) {
	client := NewMockIPFSClient()
	ctx := context.Background()

	t.Run("BasicOperations", func(t *testing.T) {
		testBlock := &blocks.Block{
			Data: []byte("test data for mock client"),
		}

		// Test Put operation
		address, err := client.Put(ctx, testBlock)
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}
		if address == nil {
			t.Fatal("Put returned nil address")
		}
		if address.ID == "" {
			t.Fatal("Put returned empty ID")
		}

		// Test Has operation
		exists, err := client.Has(ctx, address)
		if err != nil {
			t.Fatalf("Has failed: %v", err)
		}
		if !exists {
			t.Fatal("Has returned false for existing block")
		}

		// Test Get operation
		retrievedBlock, err := client.Get(ctx, address)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if string(retrievedBlock.Data) != string(testBlock.Data) {
			t.Fatalf("Retrieved data mismatch: got %s, want %s", retrievedBlock.Data, testBlock.Data)
		}

		// Test Delete operation
		err = client.Delete(ctx, address)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		// Verify deletion
		exists, err = client.Has(ctx, address)
		if err != nil {
			t.Fatalf("Has after delete failed: %v", err)
		}
		if exists {
			t.Fatal("Has returned true for deleted block")
		}
	})

	t.Run("PeerAwareOperations", func(t *testing.T) {
		testBlock := &blocks.Block{
			Data: []byte("test data for peer-aware operations"),
		}

		address, err := client.Put(ctx, testBlock)
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		// Test GetWithPeerHint
		peers := []string{"peer1", "peer2"}
		retrievedBlock, err := client.GetWithPeerHint(ctx, address, peers)
		if err != nil {
			t.Fatalf("GetWithPeerHint failed: %v", err)
		}
		if string(retrievedBlock.Data) != string(testBlock.Data) {
			t.Fatalf("Retrieved data mismatch: got %s, want %s", retrievedBlock.Data, testBlock.Data)
		}

		// Test BroadcastToNetwork
		err = client.BroadcastToNetwork(ctx, address, testBlock)
		if err != nil {
			t.Fatalf("BroadcastToNetwork failed: %v", err)
		}

		// Test GetConnectedPeers
		connectedPeers := client.GetConnectedPeers()
		if len(connectedPeers) == 0 {
			t.Fatal("GetConnectedPeers returned empty list")
		}
	})

	t.Run("ErrorSimulation", func(t *testing.T) {
		// Test store error simulation
		expectedErr := fmt.Errorf("simulated store error")
		client.SetErrorMode("put", expectedErr)

		testBlock := &blocks.Block{
			Data: []byte("test data for error simulation"),
		}

		_, err := client.Put(ctx, testBlock)
		if err == nil {
			t.Fatal("Put should have failed with simulated error")
		}

		// Clear error mode
		client.ClearErrorMode("put")

		// Should work now
		_, err = client.Put(ctx, testBlock)
		if err != nil {
			t.Fatalf("Put failed after clearing error mode: %v", err)
		}
	})

	t.Run("LatencySimulation", func(t *testing.T) {
		latency := time.Millisecond * 100
		client.SetLatency(latency)

		testBlock := &blocks.Block{
			Data: []byte("test data for latency simulation"),
		}

		start := time.Now()
		_, err := client.Put(ctx, testBlock)
		duration := time.Since(start)

		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}
		if duration < latency {
			t.Fatalf("Operation completed too quickly: %v < %v", duration, latency)
		}

		// Reset latency
		client.SetLatency(0)
	})

	t.Run("StorageQuota", func(t *testing.T) {
		// Set small quota
		client.SetStorageQuota(100) // 100 bytes

		// Try to store large block
		largeBlock := &blocks.Block{
			Data: make([]byte, 200), // 200 bytes
		}

		_, err := client.Put(ctx, largeBlock)
		if err == nil {
			t.Fatal("Put should have failed due to quota exceeded")
		}

		// Check error type
		if storageErr, ok := err.(*storage.StorageError); ok {
			if storageErr.Code != storage.ErrCodeQuotaExceeded {
				t.Fatalf("Expected quota exceeded error, got: %s", storageErr.Code)
			}
		} else {
			t.Fatalf("Expected StorageError, got: %T", err)
		}
	})

	t.Run("ConnectionManagement", func(t *testing.T) {
		// Test connection status
		if !client.IsConnected() {
			t.Fatal("Client should be connected by default")
		}

		// Test disconnect
		err := client.Disconnect(ctx)
		if err != nil {
			t.Fatalf("Disconnect failed: %v", err)
		}
		if client.IsConnected() {
			t.Fatal("Client should be disconnected")
		}

		// Test operations while disconnected
		testBlock := &blocks.Block{
			Data: []byte("test data while disconnected"),
		}
		_, err = client.Put(ctx, testBlock)
		if err == nil {
			t.Fatal("Put should fail when disconnected")
		}

		// Test reconnect
		err = client.Connect(ctx)
		if err != nil {
			t.Fatalf("Connect failed: %v", err)
		}
		if !client.IsConnected() {
			t.Fatal("Client should be connected")
		}
	})

	t.Run("MetricsTracking", func(t *testing.T) {
		client.Reset() // Reset metrics

		testBlock := &blocks.Block{
			Data: []byte("test data for metrics"),
		}

		// Perform operations
		address, _ := client.Put(ctx, testBlock)
		client.Get(ctx, address)
		client.Has(ctx, address)

		// Check metrics
		metrics := client.GetOperationCounts()
		if metrics["put"] != 1 {
			t.Fatalf("Expected 1 put operation, got %d", metrics["put"])
		}
		if metrics["get"] != 1 {
			t.Fatalf("Expected 1 get operation, got %d", metrics["get"])
		}
		if metrics["has"] != 1 {
			t.Fatalf("Expected 1 has operation, got %d", metrics["has"])
		}

		// Check operation history
		history := client.GetOperationHistory()
		if len(history) != 3 {
			t.Fatalf("Expected 3 operations in history, got %d", len(history))
		}
	})
}

// TestNetworkSimulator validates the NetworkSimulator implementation
func TestNetworkSimulator(t *testing.T) {
	sim := NewNetworkSimulator()

	t.Run("PeerManagement", func(t *testing.T) {
		// Add peers
		peer1 := sim.AddPeer("peer1")
		if peer1 == nil {
			t.Fatal("AddPeer returned nil")
		}
		if !peer1.Online {
			t.Fatal("New peer should be online")
		}

		peer2 := sim.AddPeer("peer2")
		if peer2 == nil {
			t.Fatal("AddPeer returned nil for second peer")
		}

		// Check network stats
		stats := sim.GetNetworkStats()
		if stats["total_peers"].(int) != 2 {
			t.Fatalf("Expected 2 total peers, got %v", stats["total_peers"])
		}
		if stats["online_peers"].(int) != 2 {
			t.Fatalf("Expected 2 online peers, got %v", stats["online_peers"])
		}

		// Take peer offline
		sim.SetPeerOnline("peer1", false)
		stats = sim.GetNetworkStats()
		if stats["online_peers"].(int) != 1 {
			t.Fatalf("Expected 1 online peer after taking one offline, got %v", stats["online_peers"])
		}

		// Remove peer
		sim.RemovePeer("peer2")
		stats = sim.GetNetworkStats()
		if stats["total_peers"].(int) != 1 {
			t.Fatalf("Expected 1 total peer after removal, got %v", stats["total_peers"])
		}
	})

	t.Run("ConnectionManagement", func(t *testing.T) {
		sim.Reset()
		sim.AddPeer("peer1")
		sim.AddPeer("peer2")

		// Establish connection
		err := sim.EstablishConnection("peer1", "peer2")
		if err != nil {
			t.Fatalf("EstablishConnection failed: %v", err)
		}

		// Check connection count
		stats := sim.GetNetworkStats()
		if stats["total_connections"].(int) != 1 {
			t.Fatalf("Expected 1 connection, got %v", stats["total_connections"])
		}

		// Try to connect offline peer
		sim.SetPeerOnline("peer2", false)
		err = sim.EstablishConnection("peer1", "peer2")
		if err == nil {
			t.Fatal("Should not be able to connect to offline peer")
		}
	})

	t.Run("NetworkConditions", func(t *testing.T) {
		// Test latency configuration
		baseLatency := time.Millisecond * 100
		variance := time.Millisecond * 20
		sim.SetNetworkLatency(baseLatency, variance)

		stats := sim.GetNetworkStats()
		if stats["base_latency"].(time.Duration) != baseLatency {
			t.Fatalf("Expected base latency %v, got %v", baseLatency, stats["base_latency"])
		}

		// Test packet loss configuration
		packetLoss := 0.05 // 5%
		sim.SetPacketLossRate(packetLoss)

		stats = sim.GetNetworkStats()
		if stats["packet_loss_rate"].(float64) != packetLoss {
			t.Fatalf("Expected packet loss rate %v, got %v", packetLoss, stats["packet_loss_rate"])
		}

		// Test bandwidth configuration
		bandwidth := int64(500000) // 500KB/s
		sim.SetBandwidthLimit(bandwidth)

		stats = sim.GetNetworkStats()
		if stats["bandwidth_limit"].(int64) != bandwidth {
			t.Fatalf("Expected bandwidth limit %v, got %v", bandwidth, stats["bandwidth_limit"])
		}
	})

	t.Run("NetworkPartitioning", func(t *testing.T) {
		sim.Reset()
		sim.AddPeer("peer1")
		sim.AddPeer("peer2")
		sim.AddPeer("peer3")

		// Create network partition
		partitionPeers := []string{"peer1", "peer2"}
		sim.CreateNetworkPartition(partitionPeers)

		stats := sim.GetNetworkStats()
		if stats["network_partitions"].(int) != 2 {
			t.Fatalf("Expected 2 partitioned peers, got %v", stats["network_partitions"])
		}

		// Heal partition
		sim.HealNetworkPartition()

		stats = sim.GetNetworkStats()
		if stats["network_partitions"].(int) != 0 {
			t.Fatalf("Expected 0 partitioned peers after healing, got %v", stats["network_partitions"])
		}
	})

	t.Run("EventHistory", func(t *testing.T) {
		sim.Reset()
		sim.ClearEventHistory()

		// Perform operations that generate events
		sim.AddPeer("peer1")
		sim.AddPeer("peer2")
		sim.EstablishConnection("peer1", "peer2")

		// Check event history
		events := sim.GetEventHistory()
		if len(events) < 3 { // At least peer_join, peer_join, connection_established
			t.Fatalf("Expected at least 3 events, got %d", len(events))
		}

		// Verify event types
		eventTypes := make(map[string]int)
		for _, event := range events {
			eventTypes[event.Type]++
		}

		if eventTypes["peer_join"] != 2 {
			t.Fatalf("Expected 2 peer_join events, got %d", eventTypes["peer_join"])
		}
		if eventTypes["connection_established"] != 1 {
			t.Fatalf("Expected 1 connection_established event, got %d", eventTypes["connection_established"])
		}
	})
}

// TestConditionSimulator validates the ConditionSimulator implementation
func TestConditionSimulator(t *testing.T) {
	mockClient := NewMockIPFSClient()
	networkSim := NewNetworkSimulator()
	conditionSim := NewConditionSimulator(mockClient, networkSim)

	t.Run("PredefinedConditions", func(t *testing.T) {
		conditions := conditionSim.GetPredefinedConditions()
		
		// Check that predefined conditions exist
		expectedConditions := []string{
			"high_latency", "packet_loss", "bandwidth_limit",
			"peer_churn", "dht_partitioning", "storage_pressure",
			"random_failures", "byzantine_peers", "connection_instability",
			"slow_peer_response",
		}

		for _, expected := range expectedConditions {
			if _, exists := conditions[expected]; !exists {
				t.Fatalf("Predefined condition %s not found", expected)
			}
		}

		// Verify condition structure
		highLatency := conditions["high_latency"]
		if highLatency.Type != "network" {
			t.Fatalf("Expected network type, got %s", highLatency.Type)
		}
		if highLatency.Severity != "medium" {
			t.Fatalf("Expected medium severity, got %s", highLatency.Severity)
		}
		if len(highLatency.Effects) == 0 {
			t.Fatal("Expected effects to be defined")
		}
	})

	t.Run("ConditionApplication", func(t *testing.T) {
		conditionSim.Start()
		defer conditionSim.Stop()

		// Apply a condition
		err := conditionSim.ApplyCondition("high_latency")
		if err != nil {
			t.Fatalf("ApplyCondition failed: %v", err)
		}

		// Check that condition is active
		activeConditions := conditionSim.GetActiveConditions()
		if len(activeConditions) != 1 {
			t.Fatalf("Expected 1 active condition, got %d", len(activeConditions))
		}

		if _, exists := activeConditions["high_latency"]; !exists {
			t.Fatal("high_latency condition should be active")
		}

		// Remove condition
		err = conditionSim.RemoveCondition("high_latency")
		if err != nil {
			t.Fatalf("RemoveCondition failed: %v", err)
		}

		// Check that condition is no longer active
		activeConditions = conditionSim.GetActiveConditions()
		if len(activeConditions) != 0 {
			t.Fatalf("Expected 0 active conditions, got %d", len(activeConditions))
		}
	})

	t.Run("ScheduledConditions", func(t *testing.T) {
		// Schedule a condition
		delay := time.Millisecond * 100
		err := conditionSim.ScheduleCondition("packet_loss", delay)
		if err != nil {
			t.Fatalf("ScheduleCondition failed: %v", err)
		}

		// Start simulator to process queue
		conditionSim.Start()
		defer conditionSim.Stop()

		// Wait for condition to be applied
		time.Sleep(delay + time.Millisecond*50)

		// Check that condition is active
		activeConditions := conditionSim.GetActiveConditions()
		if len(activeConditions) != 1 {
			t.Fatalf("Expected 1 active condition, got %d", len(activeConditions))
		}
	})

	t.Run("ScenarioApplication", func(t *testing.T) {
		conditionSim.Start()
		defer conditionSim.Stop()

		// Apply a scenario
		err := conditionSim.ApplyScenario("network_stress")
		if err != nil {
			t.Fatalf("ApplyScenario failed: %v", err)
		}

		// Wait for initial condition to be applied
		time.Sleep(time.Millisecond * 50)

		// Check that at least one condition is active
		activeConditions := conditionSim.GetActiveConditions()
		if len(activeConditions) == 0 {
			t.Fatal("Expected at least one active condition from scenario")
		}
	})

	t.Run("ConditionStats", func(t *testing.T) {
		stats := conditionSim.GetConditionStats()

		// Check that stats contain expected fields
		expectedFields := []string{
			"active_conditions", "queued_conditions",
			"applied_conditions", "failed_conditions",
			"simulator_running", "history_size",
		}

		for _, field := range expectedFields {
			if _, exists := stats[field]; !exists {
				t.Fatalf("Expected stats field %s not found", field)
			}
		}
	})
}

// TestMockBackendAdapter validates the MockBackendAdapter implementation
func TestMockBackendAdapter(t *testing.T) {
	t.Run("UnitTestAdapter", func(t *testing.T) {
		adapter := NewUnitTestAdapter()
		defer adapter.Stop()

		ctx := context.Background()
		testBlock := &blocks.Block{
			Data: []byte("test data for unit test adapter"),
		}

		// Test basic operations
		address, err := adapter.Put(ctx, testBlock)
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		retrievedBlock, err := adapter.Get(ctx, address)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if string(retrievedBlock.Data) != string(testBlock.Data) {
			t.Fatalf("Data mismatch: got %s, want %s", retrievedBlock.Data, testBlock.Data)
		}

		// Test adapter stats
		stats := adapter.GetAdapterStats()
		if stats["test_mode"] != "unit" {
			t.Fatalf("Expected unit test mode, got %v", stats["test_mode"])
		}
	})

	t.Run("IntegrationTestAdapter", func(t *testing.T) {
		adapter := NewIntegrationTestAdapter()
		defer adapter.Stop()

		ctx := context.Background()
		testBlock := &blocks.Block{
			Data: []byte("test data for integration test adapter"),
		}

		// Test peer-aware operations
		address, err := adapter.Put(ctx, testBlock)
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		peers := []string{"peer1", "peer2"}
		retrievedBlock, err := adapter.GetWithPeerHint(ctx, address, peers)
		if err != nil {
			t.Fatalf("GetWithPeerHint failed: %v", err)
		}

		if string(retrievedBlock.Data) != string(testBlock.Data) {
			t.Fatalf("Data mismatch: got %s, want %s", retrievedBlock.Data, testBlock.Data)
		}

		// Test condition application
		err = adapter.ApplyTestCondition("high_latency")
		if err != nil {
			t.Fatalf("ApplyTestCondition failed: %v", err)
		}

		// Test network simulation
		adapter.SimulateNetworkPartition([]string{"peer1"})
		adapter.HealNetworkPartition()

		stats := adapter.GetAdapterStats()
		if stats["test_mode"] != "integration" {
			t.Fatalf("Expected integration test mode, got %v", stats["test_mode"])
		}
	})

	t.Run("E2ETestAdapter", func(t *testing.T) {
		adapter := NewE2ETestAdapter()
		defer adapter.Stop()

		// Test scenario application
		err := adapter.ApplyTestScenario("network_stress")
		if err != nil {
			t.Fatalf("ApplyTestScenario failed: %v", err)
		}

		// Test health check
		ctx := context.Background()
		health := adapter.HealthCheck(ctx)
		if health == nil {
			t.Fatal("HealthCheck returned nil")
		}

		stats := adapter.GetAdapterStats()
		if stats["test_mode"] != "e2e" {
			t.Fatalf("Expected e2e test mode, got %v", stats["test_mode"])
		}
	})


	t.Run("ComponentAccess", func(t *testing.T) {
		adapter := NewIntegrationTestAdapter()
		defer adapter.Stop()

		// Test direct component access
		mockClient := adapter.GetMockClient()
		if mockClient == nil {
			t.Fatal("GetMockClient returned nil")
		}

		networkSim := adapter.GetNetworkSimulator()
		if networkSim == nil {
			t.Fatal("GetNetworkSimulator returned nil")
		}

		conditionSim := adapter.GetConditionSimulator()
		if conditionSim == nil {
			t.Fatal("GetConditionSimulator returned nil")
		}

		// Test direct configuration
		mockClient.SetLatency(time.Millisecond * 200)
		networkSim.SetPacketLossRate(0.1)

		// Verify configuration took effect
		if mockClient.latency != time.Millisecond*200 {
			t.Fatalf("Expected latency %v, got %v", time.Millisecond*200, mockClient.latency)
		}
	})
}

// BenchmarkMockComponents benchmarks the mock infrastructure performance
func BenchmarkMockComponents(b *testing.B) {
	b.Run("MockIPFSClient", func(b *testing.B) {
		client := NewMockIPFSClient()
		ctx := context.Background()
		testBlock := &blocks.Block{
			Data: []byte("benchmark test data"),
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			address, err := client.Put(ctx, testBlock)
			if err != nil {
				b.Fatalf("Put failed: %v", err)
			}
			_, err = client.Get(ctx, address)
			if err != nil {
				b.Fatalf("Get failed: %v", err)
			}
		}
	})

	b.Run("NetworkSimulator", func(b *testing.B) {
		sim := NewNetworkSimulator()
		
		// Pre-populate with peers
		for i := 0; i < 10; i++ {
			sim.AddPeer(fmt.Sprintf("peer%d", i))
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sim.EstablishConnection(fmt.Sprintf("peer%d", i%5), fmt.Sprintf("peer%d", (i+1)%5))
		}
	})

	b.Run("ConditionSimulator", func(b *testing.B) {
		mockClient := NewMockIPFSClient()
		networkSim := NewNetworkSimulator()
		conditionSim := NewConditionSimulator(mockClient, networkSim)
		conditionSim.Start()
		defer conditionSim.Stop()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if i%3 == 0 {
				conditionSim.ApplyCondition("high_latency")
			} else if i%3 == 1 {
				conditionSim.ApplyCondition("packet_loss")
			} else {
				conditionSim.RemoveCondition("high_latency")
			}
		}
	})

	b.Run("MockBackendAdapter", func(b *testing.B) {
		adapter := NewUnitTestAdapter()
		defer adapter.Stop()
		
		ctx := context.Background()
		testBlock := &blocks.Block{
			Data: []byte("adapter benchmark test data"),
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			address, err := adapter.Put(ctx, testBlock)
			if err != nil {
				b.Fatalf("Put failed: %v", err)
			}
			_, err = adapter.Get(ctx, address)
			if err != nil {
				b.Fatalf("Get failed: %v", err)
			}
		}
	})
}

// Integration test demonstrating complete mock infrastructure usage
func TestMockInfrastructureIntegration(t *testing.T) {
	// Create integration test adapter
	adapter := NewIntegrationTestAdapter()
	defer adapter.Stop()

	ctx := context.Background()

	t.Run("CompleteWorkflow", func(t *testing.T) {
		// Apply network stress scenario
		err := adapter.ApplyTestScenario("network_stress")
		if err != nil {
			t.Fatalf("Failed to apply scenario: %v", err)
		}

		// Wait for initial conditions
		time.Sleep(time.Millisecond * 100)

		// Perform operations under stress
		testData := [][]byte{
			[]byte("first test block"),
			[]byte("second test block"),
			[]byte("third test block"),
		}

		addresses := make([]*storage.BlockAddress, len(testData))
		
		// Store blocks
		for i, data := range testData {
			block := &blocks.Block{Data: data}
			address, err := adapter.Put(ctx, block)
			if err != nil {
				t.Fatalf("Put failed for block %d: %v", i, err)
			}
			addresses[i] = address
		}

		// Retrieve blocks with peer hints
		for i, address := range addresses {
			peers := []string{"peer1", "peer2", "peer3"}
			block, err := adapter.GetWithPeerHint(ctx, address, peers)
			if err != nil {
				t.Fatalf("GetWithPeerHint failed for block %d: %v", i, err)
			}
			
			if string(block.Data) != string(testData[i]) {
				t.Fatalf("Data mismatch for block %d: got %s, want %s", 
					i, block.Data, testData[i])
			}
		}

		// Test broadcast operations
		for _, address := range addresses {
			block, _ := adapter.Get(ctx, address)
			err := adapter.BroadcastToNetwork(ctx, address, block)
			if err != nil {
				t.Fatalf("BroadcastToNetwork failed: %v", err)
			}
		}

		// Check adapter statistics
		stats := adapter.GetAdapterStats()
		adapterCalls := stats["adapter_calls"].(map[string]int64)
		
		if adapterCalls["Put"] != int64(len(testData)) {
			t.Fatalf("Expected %d Put calls, got %d", len(testData), adapterCalls["Put"])
		}
		
		// Verify health check under stress
		health := adapter.HealthCheck(ctx)
		if !health.Healthy {
			t.Logf("System unhealthy under stress (expected): %s", health.Status)
		}

		// Apply recovery scenario
		err = adapter.RemoveTestCondition("high_latency")
		if err != nil {
			t.Fatalf("Failed to remove condition: %v", err)
		}

		// Wait for recovery
		time.Sleep(time.Millisecond * 100)

		// Verify recovery
		health = adapter.HealthCheck(ctx)
		if !health.Healthy {
			t.Logf("System still recovering: %s", health.Status)
		}
	})
}