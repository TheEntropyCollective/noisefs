package testing

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// ConditionSimulator provides comprehensive test condition simulation for IPFS
type ConditionSimulator struct {
	mu sync.RWMutex

	// Active conditions
	activeConditions map[string]*TestCondition
	conditionQueue   []*QueuedCondition
	
	// Simulation state
	isRunning        bool
	stopChannel      chan bool
	
	// Integrations
	mockClient       *MockIPFSClient
	networkSim       *NetworkSimulator
	
	// Metrics
	appliedConditions int64
	failedConditions  int64
	conditionHistory  []ConditionEvent
}

// TestCondition represents a test condition that can be applied
type TestCondition struct {
	ID          string
	Name        string
	Type        string
	Description string
	Severity    string // "low", "medium", "high", "critical"
	Duration    time.Duration
	StartTime   time.Time
	EndTime     time.Time
	Active      bool
	Parameters  map[string]interface{}
	Effects     []ConditionEffect
}

// ConditionEffect represents the effect of a test condition
type ConditionEffect struct {
	Target     string      // "latency", "errors", "bandwidth", etc.
	Action     string      // "increase", "decrease", "set", "multiply"
	Value      interface{} // The value to apply
	Probability float64    // Probability of effect occurring (0.0 to 1.0)
}

// QueuedCondition represents a condition scheduled for future execution
type QueuedCondition struct {
	Condition *TestCondition
	StartAt   time.Time
	Executed  bool
}

// ConditionEvent represents a condition application event
type ConditionEvent struct {
	Timestamp   time.Time
	ConditionID string
	Action      string // "applied", "removed", "failed"
	Details     map[string]interface{}
	Success     bool
	Error       string
}

// NewConditionSimulator creates a new condition simulator
func NewConditionSimulator(mockClient *MockIPFSClient, networkSim *NetworkSimulator) *ConditionSimulator {
	return &ConditionSimulator{
		activeConditions: make(map[string]*TestCondition),
		conditionQueue:   make([]*QueuedCondition, 0),
		stopChannel:      make(chan bool),
		mockClient:       mockClient,
		networkSim:       networkSim,
		conditionHistory: make([]ConditionEvent, 0),
	}
}

// Predefined Test Conditions

// GetPredefinedConditions returns a set of common IPFS test conditions
func (c *ConditionSimulator) GetPredefinedConditions() map[string]*TestCondition {
	conditions := map[string]*TestCondition{
		"high_latency": {
			ID:          "high_latency",
			Name:        "High Network Latency",
			Type:        "network",
			Description: "Simulates high network latency conditions",
			Severity:    "medium",
			Parameters: map[string]interface{}{
				"base_latency":    time.Millisecond * 500,
				"latency_variance": time.Millisecond * 200,
			},
			Effects: []ConditionEffect{
				{Target: "latency", Action: "set", Value: time.Millisecond * 500, Probability: 1.0},
			},
		},
		"packet_loss": {
			ID:          "packet_loss",
			Name:        "Packet Loss",
			Type:        "network",
			Description: "Simulates network packet loss",
			Severity:    "high",
			Parameters: map[string]interface{}{
				"loss_rate": 0.15, // 15% packet loss
			},
			Effects: []ConditionEffect{
				{Target: "packet_loss", Action: "set", Value: 0.15, Probability: 1.0},
			},
		},
		"bandwidth_limit": {
			ID:          "bandwidth_limit",
			Name:        "Limited Bandwidth",
			Type:        "network",
			Description: "Simulates bandwidth constraints",
			Severity:    "medium",
			Parameters: map[string]interface{}{
				"bandwidth": int64(100000), // 100KB/s
			},
			Effects: []ConditionEffect{
				{Target: "bandwidth", Action: "set", Value: int64(100000), Probability: 1.0},
			},
		},
		"peer_churn": {
			ID:          "peer_churn",
			Name:        "High Peer Churn",
			Type:        "peer",
			Description: "Simulates frequent peer connections and disconnections",
			Severity:    "high",
			Parameters: map[string]interface{}{
				"churn_rate":     0.3, // 30% of peers join/leave
				"churn_interval": time.Second * 10,
			},
			Effects: []ConditionEffect{
				{Target: "peer_stability", Action: "decrease", Value: 0.3, Probability: 1.0},
			},
		},
		"dht_partitioning": {
			ID:          "dht_partitioning",
			Name:        "DHT Network Partitioning",
			Type:        "dht",
			Description: "Simulates DHT network partitions",
			Severity:    "critical",
			Parameters: map[string]interface{}{
				"partition_size": 0.4, // 40% of network partitioned
			},
			Effects: []ConditionEffect{
				{Target: "dht_reachability", Action: "decrease", Value: 0.4, Probability: 1.0},
			},
		},
		"storage_pressure": {
			ID:          "storage_pressure",
			Name:        "Storage Pressure",
			Type:        "storage",
			Description: "Simulates storage capacity constraints",
			Severity:    "medium",
			Parameters: map[string]interface{}{
				"available_storage": int64(10000000), // 10MB limit
			},
			Effects: []ConditionEffect{
				{Target: "storage_quota", Action: "set", Value: int64(10000000), Probability: 1.0},
			},
		},
		"random_failures": {
			ID:          "random_failures",
			Name:        "Random Operation Failures",
			Type:        "reliability",
			Description: "Simulates random operation failures",
			Severity:    "medium",
			Parameters: map[string]interface{}{
				"failure_rate": 0.05, // 5% operation failure rate
			},
			Effects: []ConditionEffect{
				{Target: "operation_reliability", Action: "decrease", Value: 0.05, Probability: 1.0},
			},
		},
		"byzantine_peers": {
			ID:          "byzantine_peers",
			Name:        "Byzantine Peer Behavior",
			Type:        "security",
			Description: "Simulates malicious or faulty peer behavior",
			Severity:    "critical",
			Parameters: map[string]interface{}{
				"byzantine_ratio": 0.2, // 20% byzantine peers
			},
			Effects: []ConditionEffect{
				{Target: "peer_reliability", Action: "decrease", Value: 0.2, Probability: 1.0},
			},
		},
		"connection_instability": {
			ID:          "connection_instability",
			Name:        "Unstable Connections",
			Type:        "network",
			Description: "Simulates unstable peer connections",
			Severity:    "high",
			Parameters: map[string]interface{}{
				"instability_rate": 0.25, // 25% of connections unstable
				"reconnect_delay":  time.Second * 5,
			},
			Effects: []ConditionEffect{
				{Target: "connection_stability", Action: "decrease", Value: 0.25, Probability: 1.0},
			},
		},
		"slow_peer_response": {
			ID:          "slow_peer_response",
			Name:        "Slow Peer Responses",
			Type:        "performance",
			Description: "Simulates slow peer response times",
			Severity:    "medium",
			Parameters: map[string]interface{}{
				"response_delay": time.Second * 2,
				"affected_ratio": 0.3, // 30% of peers affected
			},
			Effects: []ConditionEffect{
				{Target: "peer_response_time", Action: "increase", Value: time.Second * 2, Probability: 0.3},
			},
		},
	}

	// Set default duration for all conditions
	for _, condition := range conditions {
		if condition.Duration == 0 {
			condition.Duration = time.Minute * 5 // Default 5 minutes
		}
	}

	return conditions
}

// Condition Management

// ApplyCondition applies a test condition immediately
func (c *ConditionSimulator) ApplyCondition(conditionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	predefined := c.GetPredefinedConditions()
	condition, exists := predefined[conditionID]
	if !exists {
		return fmt.Errorf("condition %s not found", conditionID)
	}

	// Clone the condition to avoid modifying the original
	activeCondition := c.cloneCondition(condition)
	activeCondition.StartTime = time.Now()
	activeCondition.EndTime = activeCondition.StartTime.Add(activeCondition.Duration)
	activeCondition.Active = true

	// Apply the condition effects
	err := c.applyConditionEffects(activeCondition)
	if err != nil {
		c.failedConditions++
		c.recordConditionEvent(conditionID, "failed", map[string]interface{}{
			"error": err.Error(),
		}, false, err.Error())
		return err
	}

	c.activeConditions[conditionID] = activeCondition
	c.appliedConditions++
	
	c.recordConditionEvent(conditionID, "applied", map[string]interface{}{
		"duration": activeCondition.Duration,
		"severity": activeCondition.Severity,
	}, true, "")

	// Schedule automatic removal
	go c.scheduleConditionRemoval(conditionID, activeCondition.Duration)

	return nil
}

// RemoveCondition removes an active test condition
func (c *ConditionSimulator) RemoveCondition(conditionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	condition, exists := c.activeConditions[conditionID]
	if !exists {
		return fmt.Errorf("condition %s is not active", conditionID)
	}

	err := c.removeConditionEffects(condition)
	if err != nil {
		c.recordConditionEvent(conditionID, "removal_failed", map[string]interface{}{
			"error": err.Error(),
		}, false, err.Error())
		return err
	}

	delete(c.activeConditions, conditionID)
	
	c.recordConditionEvent(conditionID, "removed", map[string]interface{}{
		"actual_duration": time.Since(condition.StartTime),
	}, true, "")

	return nil
}

// ScheduleCondition schedules a condition to be applied at a future time
func (c *ConditionSimulator) ScheduleCondition(conditionID string, delay time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	predefined := c.GetPredefinedConditions()
	condition, exists := predefined[conditionID]
	if !exists {
		return fmt.Errorf("condition %s not found", conditionID)
	}

	queuedCondition := &QueuedCondition{
		Condition: c.cloneCondition(condition),
		StartAt:   time.Now().Add(delay),
		Executed:  false,
	}

	c.conditionQueue = append(c.conditionQueue, queuedCondition)
	return nil
}

// Scenario Management

// ApplyScenario applies a predefined test scenario
func (c *ConditionSimulator) ApplyScenario(scenarioName string) error {
	scenarios := c.getPredefinedScenarios()
	scenario, exists := scenarios[scenarioName]
	if !exists {
		return fmt.Errorf("scenario %s not found", scenarioName)
	}

	for _, step := range scenario {
		err := c.ScheduleCondition(step.ConditionID, step.Delay)
		if err != nil {
			return fmt.Errorf("failed to schedule condition %s: %v", step.ConditionID, err)
		}
	}

	return nil
}

type ScenarioStep struct {
	ConditionID string
	Delay       time.Duration
}

func (c *ConditionSimulator) getPredefinedScenarios() map[string][]ScenarioStep {
	return map[string][]ScenarioStep{
		"network_stress": {
			{ConditionID: "high_latency", Delay: 0},
			{ConditionID: "packet_loss", Delay: time.Second * 30},
			{ConditionID: "bandwidth_limit", Delay: time.Minute * 1},
		},
		"peer_instability": {
			{ConditionID: "peer_churn", Delay: 0},
			{ConditionID: "connection_instability", Delay: time.Second * 10},
			{ConditionID: "slow_peer_response", Delay: time.Second * 30},
		},
		"security_stress": {
			{ConditionID: "byzantine_peers", Delay: 0},
			{ConditionID: "dht_partitioning", Delay: time.Second * 20},
		},
		"resource_pressure": {
			{ConditionID: "storage_pressure", Delay: 0},
			{ConditionID: "bandwidth_limit", Delay: time.Second * 15},
			{ConditionID: "random_failures", Delay: time.Second * 30},
		},
	}
}

// Simulator Control

// Start starts the condition simulator
func (c *ConditionSimulator) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isRunning {
		return
	}

	c.isRunning = true
	go c.run()
}

// Stop stops the condition simulator
func (c *ConditionSimulator) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isRunning {
		return
	}

	c.isRunning = false
	c.stopChannel <- true

	// Remove all active conditions
	for conditionID := range c.activeConditions {
		c.RemoveCondition(conditionID)
	}
}

// Status and Monitoring

// GetActiveConditions returns currently active conditions
func (c *ConditionSimulator) GetActiveConditions() map[string]*TestCondition {
	c.mu.RLock()
	defer c.mu.RUnlock()

	active := make(map[string]*TestCondition)
	for id, condition := range c.activeConditions {
		active[id] = c.cloneCondition(condition)
	}
	return active
}

// GetConditionStats returns condition statistics
func (c *ConditionSimulator) GetConditionStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"active_conditions":  len(c.activeConditions),
		"queued_conditions":  len(c.conditionQueue),
		"applied_conditions": c.appliedConditions,
		"failed_conditions":  c.failedConditions,
		"simulator_running":  c.isRunning,
		"history_size":       len(c.conditionHistory),
	}
}

// GetConditionHistory returns condition event history
func (c *ConditionSimulator) GetConditionHistory() []ConditionEvent {
	c.mu.RLock()
	defer c.mu.RUnlock()

	history := make([]ConditionEvent, len(c.conditionHistory))
	copy(history, c.conditionHistory)
	return history
}

// Condition Effect Application

func (c *ConditionSimulator) applyConditionEffects(condition *TestCondition) error {
	for _, effect := range condition.Effects {
		if rand.Float64() > effect.Probability {
			continue // Skip this effect based on probability
		}

		err := c.applyEffect(effect)
		if err != nil {
			return fmt.Errorf("failed to apply effect %s: %v", effect.Target, err)
		}
	}
	return nil
}

func (c *ConditionSimulator) removeConditionEffects(condition *TestCondition) error {
	// Reset effects to default values
	for _, effect := range condition.Effects {
		err := c.resetEffect(effect)
		if err != nil {
			return fmt.Errorf("failed to reset effect %s: %v", effect.Target, err)
		}
	}
	return nil
}

func (c *ConditionSimulator) applyEffect(effect ConditionEffect) error {
	switch effect.Target {
	case "latency":
		if latency, ok := effect.Value.(time.Duration); ok {
			c.mockClient.SetLatency(latency)
			c.networkSim.SetNetworkLatency(latency, latency/4)
		}
	case "packet_loss":
		if rate, ok := effect.Value.(float64); ok {
			c.networkSim.SetPacketLossRate(rate)
		}
	case "bandwidth":
		if bandwidth, ok := effect.Value.(int64); ok {
			c.mockClient.SetBandwidthLimit(bandwidth)
			c.networkSim.SetBandwidthLimit(bandwidth)
		}
	case "storage_quota":
		if quota, ok := effect.Value.(int64); ok {
			c.mockClient.SetStorageQuota(quota)
		}
	case "operation_reliability":
		if rate, ok := effect.Value.(float64); ok {
			c.mockClient.SetFailureRate(rate)
		}
	case "peer_reliability":
		if ratio, ok := effect.Value.(float64); ok {
			c.networkSim.EnableByzantine(true, ratio)
		}
	case "dht_reachability":
		// Simulate DHT partitioning by creating network partitions
		if ratio, ok := effect.Value.(float64); ok {
			peers := c.networkSim.GetNetworkStats()["online_peers"].(int)
			partitionSize := int(float64(peers) * ratio)
			if partitionSize > 0 {
				// Create partition with random peers
				peerIDs := make([]string, partitionSize)
				for i := 0; i < partitionSize; i++ {
					peerIDs[i] = fmt.Sprintf("peer%d", i)
				}
				c.networkSim.CreateNetworkPartition(peerIDs)
			}
		}
	default:
		return fmt.Errorf("unknown effect target: %s", effect.Target)
	}
	return nil
}

func (c *ConditionSimulator) resetEffect(effect ConditionEffect) error {
	switch effect.Target {
	case "latency":
		c.mockClient.SetLatency(time.Millisecond * 50) // Default latency
		c.networkSim.SetNetworkLatency(time.Millisecond*50, time.Millisecond*20)
	case "packet_loss":
		c.networkSim.SetPacketLossRate(0.01) // Default 1%
	case "bandwidth":
		c.mockClient.SetBandwidthLimit(1000000) // Default 1MB/s
		c.networkSim.SetBandwidthLimit(1000000)
	case "storage_quota":
		c.mockClient.SetStorageQuota(1000000000) // Default 1GB
	case "operation_reliability":
		c.mockClient.SetFailureRate(0.0) // No failures
	case "peer_reliability":
		c.networkSim.EnableByzantine(false, 0.0)
	case "dht_reachability":
		c.networkSim.HealNetworkPartition()
	default:
		return fmt.Errorf("unknown effect target: %s", effect.Target)
	}
	return nil
}

// Private methods

func (c *ConditionSimulator) run() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChannel:
			return
		case <-ticker.C:
			c.processQueuedConditions()
			c.checkExpiredConditions()
		}
	}
}

func (c *ConditionSimulator) processQueuedConditions() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for _, queued := range c.conditionQueue {
		if !queued.Executed && now.After(queued.StartAt) {
			err := c.ApplyCondition(queued.Condition.ID)
			queued.Executed = true
			if err != nil {
				// Log error but continue
				continue
			}
		}
	}

	// Remove executed conditions from queue
	newQueue := make([]*QueuedCondition, 0)
	for _, queued := range c.conditionQueue {
		if !queued.Executed {
			newQueue = append(newQueue, queued)
		}
	}
	c.conditionQueue = newQueue
}

func (c *ConditionSimulator) checkExpiredConditions() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	expired := make([]string, 0)

	for conditionID, condition := range c.activeConditions {
		if now.After(condition.EndTime) {
			expired = append(expired, conditionID)
		}
	}

	for _, conditionID := range expired {
		c.RemoveCondition(conditionID)
	}
}

func (c *ConditionSimulator) scheduleConditionRemoval(conditionID string, duration time.Duration) {
	time.Sleep(duration)
	c.RemoveCondition(conditionID)
}

func (c *ConditionSimulator) cloneCondition(condition *TestCondition) *TestCondition {
	clone := *condition
	clone.Parameters = make(map[string]interface{})
	for k, v := range condition.Parameters {
		clone.Parameters[k] = v
	}
	clone.Effects = make([]ConditionEffect, len(condition.Effects))
	copy(clone.Effects, condition.Effects)
	return &clone
}

func (c *ConditionSimulator) recordConditionEvent(conditionID, action string, details map[string]interface{}, success bool, errorMsg string) {
	event := ConditionEvent{
		Timestamp:   time.Now(),
		ConditionID: conditionID,
		Action:      action,
		Details:     details,
		Success:     success,
		Error:       errorMsg,
	}

	c.conditionHistory = append(c.conditionHistory, event)

	// Limit history size
	if len(c.conditionHistory) > 1000 {
		c.conditionHistory = c.conditionHistory[100:] // Keep most recent 900 events
	}
}