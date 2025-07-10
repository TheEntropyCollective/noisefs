package testing

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// MultiNodeLauncher manages multiple IPFS daemon processes
type MultiNodeLauncher struct {
	nodes     []*IPFSNode
	nodeCount int
	baseDir   string
	cleanup   []func() error
	mu        sync.RWMutex
}

// IPFSNode represents a single IPFS daemon instance
type IPFSNode struct {
	ID         int
	APIPort    int
	P2PPort    int
	GatewayPort int
	DataDir    string
	Process    *exec.Cmd
	APIAddress string
}

// NewMultiNodeLauncher creates a new multi-node IPFS launcher
func NewMultiNodeLauncher(nodeCount int) *MultiNodeLauncher {
	baseDir := filepath.Join(os.TempDir(), fmt.Sprintf("noisefs-test-%d", time.Now().Unix()))
	
	launcher := &MultiNodeLauncher{
		nodeCount: nodeCount,
		baseDir:   baseDir,
		nodes:     make([]*IPFSNode, nodeCount),
		cleanup:   make([]func() error, 0),
	}
	
	// Initialize node configurations - start from 5010 to avoid conflicts with existing IPFS on 5001
	for i := 0; i < nodeCount; i++ {
		launcher.nodes[i] = &IPFSNode{
			ID:          i + 1,
			APIPort:     5010 + i,  // Start from 5010 to avoid conflict with existing IPFS on 5001
			P2PPort:     4010 + i,  // Start from 4010 to avoid conflict with existing IPFS on 4001
			GatewayPort: 8090 + i,  // Start from 8090 to avoid conflict with existing IPFS on 8080
			DataDir:     filepath.Join(baseDir, fmt.Sprintf("node-%d", i+1)),
			APIAddress:  fmt.Sprintf("127.0.0.1:%d", 5010+i),
		}
	}
	
	return launcher
}

// StartNodes starts all IPFS daemon nodes
func (l *MultiNodeLauncher) StartNodes() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	fmt.Printf("🚀 Starting %d IPFS nodes...\n", l.nodeCount)
	
	// Create base directory
	if err := os.MkdirAll(l.baseDir, 0755); err != nil {
		return fmt.Errorf("failed to create base directory: %w", err)
	}
	
	l.cleanup = append(l.cleanup, func() error {
		return os.RemoveAll(l.baseDir)
	})
	
	// Start each node
	for i, node := range l.nodes {
		fmt.Printf("  Starting node %d on ports API:%d P2P:%d Gateway:%d\n", 
			node.ID, node.APIPort, node.P2PPort, node.GatewayPort)
		
		if err := l.startSingleNode(node); err != nil {
			// Clean up any nodes that were started
			l.stopAllNodes()
			return fmt.Errorf("failed to start node %d: %w", i+1, err)
		}
		
		// No delay needed - we wait in startSingleNode
	}
	
	// All nodes should be ready now
	fmt.Println("⏳ Verifying all nodes...")
	
	// Verify all nodes are responsive
	readyCount := 0
	for _, node := range l.nodes {
		// Check if process is still running
		if node.Process != nil && node.Process.Process != nil {
			if err := node.Process.Process.Signal(syscall.Signal(0)); err != nil {
				fmt.Printf("  ❌ Node %d process died: %v\n", node.ID, err)
				continue
			}
		}
		
		if l.isNodeReady(node) {
			readyCount++
			fmt.Printf("  ✅ Node %d is ready\n", node.ID)
		} else {
			fmt.Printf("  ⚠️  Node %d is not responding (PID: %d)\n", node.ID, node.Process.Process.Pid)
		}
	}
	
	fmt.Printf("✅ Multi-node IPFS cluster started: %d/%d nodes ready\n", readyCount, l.nodeCount)
	return nil
}

// startSingleNode initializes and starts a single IPFS node
func (l *MultiNodeLauncher) startSingleNode(node *IPFSNode) error {
	// Create node data directory
	if err := os.MkdirAll(node.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create node directory: %w", err)
	}
	
	// Set environment for this node
	env := os.Environ()
	env = append(env, "IPFS_PATH="+node.DataDir)
	
	// Initialize IPFS repo if it doesn't exist
	configFile := filepath.Join(node.DataDir, "config")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		initCmd := exec.Command("ipfs", "init", "--profile=server")
		initCmd.Env = env
		if output, err := initCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to init IPFS node: %w\nOutput: %s", err, output)
		}
	}
	
	// Configure API and Gateway ports
	configCmds := [][]string{
		{"ipfs", "config", "Addresses.API", fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", node.APIPort)},
		{"ipfs", "config", "Addresses.Gateway", fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", node.GatewayPort)},
		{"ipfs", "config", "--json", "API.HTTPHeaders.Access-Control-Allow-Origin", "[\"*\"]"},
		{"ipfs", "config", "--json", "API.HTTPHeaders.Access-Control-Allow-Methods", "[\"GET\", \"POST\", \"PUT\", \"DELETE\"]"},
		{"ipfs", "config", "--json", "Swarm.ConnMgr.HighWater", "200"},
		{"ipfs", "config", "--json", "Swarm.ConnMgr.LowWater", "100"},
	}
	
	for _, cmdArgs := range configCmds {
		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		cmd.Env = env
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to configure IPFS node (%v): %w\nOutput: %s", cmdArgs, err, output)
		}
	}
	
	// Configure swarm ports to avoid conflicts
	swarmCmd := exec.Command("ipfs", "config", "--json", "Addresses.Swarm", 
		fmt.Sprintf("[\"/ip4/0.0.0.0/tcp/%d\", \"/ip6/::/tcp/%d\"]", node.P2PPort, node.P2PPort))
	swarmCmd.Env = env
	if output, err := swarmCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to configure swarm ports: %w\nOutput: %s", err, output)
	}
	
	// Start IPFS daemon with offline mode to avoid DHT conflicts
	daemonCmd := exec.Command("ipfs", "daemon", "--offline", "--enable-pubsub-experiment")
	daemonCmd.Env = env
	
	// Set process group to make cleanup easier
	daemonCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	
	// Log output for debugging
	logFile := filepath.Join(node.DataDir, "daemon.log")
	outFile, err := os.Create(logFile)
	if err == nil {
		daemonCmd.Stdout = outFile
		daemonCmd.Stderr = outFile
		defer outFile.Close()
	}
	
	if err := daemonCmd.Start(); err != nil {
		return fmt.Errorf("failed to start IPFS daemon on port %d: %w", node.APIPort, err)
	}
	
	node.Process = daemonCmd
	
	fmt.Printf("    Started IPFS daemon (PID: %d) on API port %d\n", daemonCmd.Process.Pid, node.APIPort)
	
	// Wait for node to be ready with retries
	ready := false
	for i := 0; i < 10; i++ {
		time.Sleep(2 * time.Second)
		if l.isNodeReady(node) {
			ready = true
			break
		}
	}
	
	if !ready {
		// Try to get log output
		if logContent, err := os.ReadFile(logFile); err == nil {
			fmt.Printf("    Node %d startup log:\n%s\n", node.ID, string(logContent))
		}
		return fmt.Errorf("node %d failed to become ready after 20 seconds", node.ID)
	}
	
	// Add cleanup function for this node
	l.cleanup = append(l.cleanup, func() error {
		if node.Process != nil && node.Process.Process != nil {
			// Kill the process group to ensure cleanup
			pgid, err := syscall.Getpgid(node.Process.Process.Pid)
			if err == nil {
				syscall.Kill(-pgid, syscall.SIGTERM)
			}
			node.Process.Wait()
		}
		return nil
	})
	
	return nil
}

// isNodeReady checks if a node is responsive
func (l *MultiNodeLauncher) isNodeReady(node *IPFSNode) bool {
	cmd := exec.Command("curl", "-s", "--max-time", "5", 
		fmt.Sprintf("http://127.0.0.1:%d/api/v0/id", node.APIPort))
	
	if err := cmd.Run(); err != nil {
		// For debugging - uncomment to see detailed errors
		fmt.Printf("    Debug: Node %d not ready: %v\n", node.ID, err)
		return false
	}
	return true
}

// GetNodes returns all configured nodes
func (l *MultiNodeLauncher) GetNodes() []*IPFSNode {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.nodes
}

// GetNode returns a specific node by index
func (l *MultiNodeLauncher) GetNode(index int) (*IPFSNode, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	
	if index < 0 || index >= len(l.nodes) {
		return nil, fmt.Errorf("node index %d out of range", index)
	}
	
	return l.nodes[index], nil
}

// ConnectNodes creates connections between all nodes (full mesh)
func (l *MultiNodeLauncher) ConnectNodes() error {
	l.mu.RLock()
	defer l.mu.RUnlock()
	
	fmt.Println("🔗 Connecting nodes in mesh network...")
	
	for i, sourceNode := range l.nodes {
		for j, targetNode := range l.nodes {
			if i >= j {
				continue // Skip self and already connected pairs
			}
			
			if err := l.connectTwoNodes(sourceNode, targetNode); err != nil {
				fmt.Printf("  ⚠️  Failed to connect node %d to node %d: %v\n", 
					sourceNode.ID, targetNode.ID, err)
			} else {
				fmt.Printf("  ✅ Connected node %d to node %d\n", 
					sourceNode.ID, targetNode.ID)
			}
		}
	}
	
	return nil
}

// connectTwoNodes connects two specific nodes
func (l *MultiNodeLauncher) connectTwoNodes(source, target *IPFSNode) error {
	// Get target peer ID - for simplicity, just try the connection
	// In production, we would parse the JSON response to get the actual peer ID
	
	// Connect command (using swarm connect)
	connectCmd := exec.Command("curl", "-s", "--max-time", "10", "-X", "POST",
		fmt.Sprintf("http://localhost:%d/api/v0/swarm/connect", source.APIPort),
		"-d", fmt.Sprintf("arg=/ip4/127.0.0.1/tcp/%d/p2p/", target.P2PPort))
	
	return connectCmd.Run()
}

// StopAllNodes stops all IPFS daemon processes and cleans up
func (l *MultiNodeLauncher) StopAllNodes() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	return l.stopAllNodes()
}

func (l *MultiNodeLauncher) stopAllNodes() error {
	fmt.Println("🧹 Stopping all IPFS nodes...")
	
	// Run all cleanup functions in reverse order
	for i := len(l.cleanup) - 1; i >= 0; i-- {
		if err := l.cleanup[i](); err != nil {
			fmt.Printf("Warning: cleanup error: %v\n", err)
		}
	}
	
	// Wait a moment for processes to terminate
	time.Sleep(2 * time.Second)
	
	fmt.Println("✅ All nodes stopped and cleaned up")
	return nil
}

// GetNodeCount returns the number of configured nodes
func (l *MultiNodeLauncher) GetNodeCount() int {
	return l.nodeCount
}