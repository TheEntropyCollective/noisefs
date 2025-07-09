#!/bin/bash

# Script to connect IPFS nodes in the test network

echo "Connecting IPFS nodes..."

# Wait for all nodes to be ready
sleep 15

# Function to get peer ID from a node
get_peer_id() {
    local api_url=$1
    curl -s -X POST "$api_url/api/v0/id" | grep -o '"ID":"[^"]*"' | cut -d'"' -f4
}

# Function to connect two nodes
connect_nodes() {
    local source_api=$1
    local target_api=$2
    local target_host=$3
    
    echo "Connecting $source_api to $target_api..."
    
    # Get target peer ID
    target_id=$(get_peer_id "$target_api")
    if [ -z "$target_id" ]; then
        echo "Failed to get peer ID from $target_api"
        return 1
    fi
    
    # Connect to target
    multiaddr="/ip4/$target_host/tcp/4001/p2p/$target_id"
    curl -s -X POST "$source_api/api/v0/swarm/connect?arg=$multiaddr"
    echo "Connected to $multiaddr"
}

# API endpoints
API1="http://ipfs-node-1:5001"
API2="http://ipfs-node-2:5001"
API3="http://ipfs-node-3:5001"
API4="http://ipfs-node-4:5001"
API5="http://ipfs-node-5:5001"

# Connect all nodes to each other (full mesh)
echo "Creating mesh network..."

# Node 1 connections
connect_nodes "$API1" "$API2" "ipfs-node-2"
connect_nodes "$API1" "$API3" "ipfs-node-3"
connect_nodes "$API1" "$API4" "ipfs-node-4"
connect_nodes "$API1" "$API5" "ipfs-node-5"

# Node 2 connections
connect_nodes "$API2" "$API3" "ipfs-node-3"
connect_nodes "$API2" "$API4" "ipfs-node-4"
connect_nodes "$API2" "$API5" "ipfs-node-5"

# Node 3 connections
connect_nodes "$API3" "$API4" "ipfs-node-4"
connect_nodes "$API3" "$API5" "ipfs-node-5"

# Node 4 connections
connect_nodes "$API4" "$API5" "ipfs-node-5"

echo "IPFS network setup complete!"

# Show network status
echo "Network status:"
for api in "$API1" "$API2" "$API3" "$API4" "$API5"; do
    echo "Node $api peers:"
    curl -s -X POST "$api/api/v0/swarm/peers" | grep -o '"Peer":"[^"]*"' | head -5
done