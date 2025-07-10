package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/TheEntropyCollective/noisefs/pkg/blocks"
)

// LegacyIPFSAdapter provides backward compatibility for old IPFS client interface
type LegacyIPFSAdapter struct {
	backend Backend
}

// NewLegacyIPFSAdapter creates an adapter for backward compatibility
func NewLegacyIPFSAdapter(backend Backend) *LegacyIPFSAdapter {
	return &LegacyIPFSAdapter{
		backend: backend,
	}
}

// StoreBlock stores a block and returns its CID (legacy interface)
func (adapter *LegacyIPFSAdapter) StoreBlock(block *blocks.Block) (string, error) {
	ctx := context.Background()
	address, err := adapter.backend.Put(ctx, block)
	if err != nil {
		return "", err
	}
	return address.ID, nil
}

// RetrieveBlock retrieves a block by CID (legacy interface)
func (adapter *LegacyIPFSAdapter) RetrieveBlock(cid string) (*blocks.Block, error) {
	ctx := context.Background()
	address := &BlockAddress{
		ID:          cid,
		BackendType: BackendTypeIPFS,
	}
	return adapter.backend.Get(ctx, address)
}

// RetrieveBlockWithPeerHint retrieves a block with peer hints (legacy interface)
func (adapter *LegacyIPFSAdapter) RetrieveBlockWithPeerHint(cid string, preferredPeers []string) (*blocks.Block, error) {
	ctx := context.Background()
	address := &BlockAddress{
		ID:          cid,
		BackendType: BackendTypeIPFS,
	}
	
	// Check if backend supports peer-aware operations
	if peerAware, ok := adapter.backend.(PeerAwareBackend); ok {
		return peerAware.GetWithPeerHint(ctx, address, preferredPeers)
	}
	
	// Fallback to standard retrieval
	return adapter.backend.Get(ctx, address)
}

// StoreBlockWithStrategy stores a block with a strategy (legacy interface)
func (adapter *LegacyIPFSAdapter) StoreBlockWithStrategy(block *blocks.Block, strategy string) (string, error) {
	ctx := context.Background()
	
	// Store the block first
	address, err := adapter.backend.Put(ctx, block)
	if err != nil {
		return "", err
	}
	
	// If backend supports peer-aware operations, broadcast
	if peerAware, ok := adapter.backend.(PeerAwareBackend); ok && strategy != "" {
		go peerAware.BroadcastToNetwork(ctx, address, block)
	}
	
	return address.ID, nil
}

// PinBlock pins a block (legacy interface)
func (adapter *LegacyIPFSAdapter) PinBlock(cid string) error {
	ctx := context.Background()
	address := &BlockAddress{
		ID:          cid,
		BackendType: BackendTypeIPFS,
	}
	return adapter.backend.Pin(ctx, address)
}

// UnpinBlock unpins a block (legacy interface)
func (adapter *LegacyIPFSAdapter) UnpinBlock(cid string) error {
	ctx := context.Background()
	address := &BlockAddress{
		ID:          cid,
		BackendType: BackendTypeIPFS,
	}
	return adapter.backend.Unpin(ctx, address)
}

// CIDToBlockAddress converts a CID string to a BlockAddress for the appropriate backend
func CIDToBlockAddress(cid string, backendType string) *BlockAddress {
	if backendType == "" {
		// Auto-detect backend type based on CID format
		backendType = detectBackendTypeFromCID(cid)
	}
	
	return &BlockAddress{
		ID:          cid,
		BackendType: backendType,
	}
}

// BlockAddressToCID extracts the CID from a BlockAddress (for legacy compatibility)
func BlockAddressToCID(address *BlockAddress) string {
	return address.ID
}

// detectBackendTypeFromCID attempts to detect the storage backend type from a CID format
func detectBackendTypeFromCID(cid string) string {
	switch {
	case strings.HasPrefix(cid, "Qm") || strings.HasPrefix(cid, "bafy"):
		// IPFS CIDv0 or CIDv1
		return BackendTypeIPFS
	case strings.HasPrefix(cid, "ar:"):
		// Arweave transaction ID
		return BackendTypeArweave
	case len(cid) == 64 && isHex(cid):
		// Might be a SHA256 hash for local storage
		return BackendTypeLocal
	default:
		// Default to IPFS for unknown formats
		return BackendTypeIPFS
	}
}

// isHex checks if a string contains only hexadecimal characters
func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// BackendFactory creates storage backends based on configuration
type BackendFactory struct {
	config *Config
}

// NewBackendFactory creates a new backend factory
func NewBackendFactory(config *Config) *BackendFactory {
	return &BackendFactory{config: config}
}

// CreateBackend creates a backend instance of the specified type
func (factory *BackendFactory) CreateBackend(backendName string) (Backend, error) {
	backendConfig, exists := factory.config.Backends[backendName]
	if !exists {
		return nil, fmt.Errorf("backend '%s' not found in configuration", backendName)
	}
	
	if !backendConfig.Enabled {
		return nil, fmt.Errorf("backend '%s' is disabled", backendName)
	}
	
	// Use the registry to create the backend
	return CreateBackend(backendConfig)
}

// CreateAllBackends creates all enabled backends
func (factory *BackendFactory) CreateAllBackends() (map[string]Backend, error) {
	backends := make(map[string]Backend)
	
	for name, config := range factory.config.Backends {
		if !config.Enabled {
			continue
		}
		
		backend, err := factory.CreateBackend(name)
		if err != nil {
			return nil, fmt.Errorf("failed to create backend '%s': %w", name, err)
		}
		
		backends[name] = backend
	}
	
	return backends, nil
}

// GetDefaultBackend returns the name of the default backend
func (factory *BackendFactory) GetDefaultBackend() string {
	return factory.config.DefaultBackend
}

// GetBackendsByPriority returns backend names sorted by priority
func (factory *BackendFactory) GetBackendsByPriority() []string {
	configs := factory.config.GetBackendsByPriority()
	names := make([]string, len(configs))
	
	for i, config := range configs {
		// Find the name for this config
		for name, c := range factory.config.Backends {
			if c == config {
				names[i] = name
				break
			}
		}
	}
	
	return names
}

// BackendCapabilityChecker helps determine which backends support specific capabilities
type BackendCapabilityChecker struct {
	backends map[string]Backend
}

// NewBackendCapabilityChecker creates a new capability checker
func NewBackendCapabilityChecker(backends map[string]Backend) *BackendCapabilityChecker {
	return &BackendCapabilityChecker{backends: backends}
}

// GetBackendsWithCapability returns backends that support a specific capability
func (checker *BackendCapabilityChecker) GetBackendsWithCapability(capability string) []string {
	var supportedBackends []string
	
	for name, backend := range checker.backends {
		info := backend.GetBackendInfo()
		for _, cap := range info.Capabilities {
			if cap == capability {
				supportedBackends = append(supportedBackends, name)
				break
			}
		}
	}
	
	return supportedBackends
}

// GetCommonCapabilities returns capabilities supported by all backends
func (checker *BackendCapabilityChecker) GetCommonCapabilities() []string {
	if len(checker.backends) == 0 {
		return []string{}
	}
	
	// Get capabilities from first backend as baseline
	var commonCaps []string
	var firstBackend Backend
	for _, backend := range checker.backends {
		firstBackend = backend
		break
	}
	
	firstInfo := firstBackend.GetBackendInfo()
	
	// Check each capability against all backends
	for _, cap := range firstInfo.Capabilities {
		supported := true
		for _, backend := range checker.backends {
			info := backend.GetBackendInfo()
			found := false
			for _, backendCap := range info.Capabilities {
				if backendCap == cap {
					found = true
					break
				}
			}
			if !found {
				supported = false
				break
			}
		}
		if supported {
			commonCaps = append(commonCaps, cap)
		}
	}
	
	return commonCaps
}

// GetAllCapabilities returns all capabilities supported by any backend
func (checker *BackendCapabilityChecker) GetAllCapabilities() []string {
	capSet := make(map[string]bool)
	
	for _, backend := range checker.backends {
		info := backend.GetBackendInfo()
		for _, cap := range info.Capabilities {
			capSet[cap] = true
		}
	}
	
	var allCaps []string
	for cap := range capSet {
		allCaps = append(allCaps, cap)
	}
	
	return allCaps
}

// IsCapabilitySupported checks if any backend supports a capability
func (checker *BackendCapabilityChecker) IsCapabilitySupported(capability string) bool {
	for _, backend := range checker.backends {
		info := backend.GetBackendInfo()
		for _, cap := range info.Capabilities {
			if cap == capability {
				return true
			}
		}
	}
	return false
}

// AddressConverter helps convert between different address formats
type AddressConverter struct{}

// NewAddressConverter creates a new address converter
func NewAddressConverter() *AddressConverter {
	return &AddressConverter{}
}

// ConvertFromLegacyCID converts a legacy CID to a BlockAddress
func (converter *AddressConverter) ConvertFromLegacyCID(cid string) *BlockAddress {
	return CIDToBlockAddress(cid, "")
}

// ConvertToLegacyCID converts a BlockAddress to a legacy CID
func (converter *AddressConverter) ConvertToLegacyCID(address *BlockAddress) string {
	return BlockAddressToCID(address)
}

// ConvertBatch converts multiple CIDs to BlockAddresses
func (converter *AddressConverter) ConvertBatch(cids []string, backendType string) []*BlockAddress {
	addresses := make([]*BlockAddress, len(cids))
	for i, cid := range cids {
		addresses[i] = CIDToBlockAddress(cid, backendType)
	}
	return addresses
}

// ConvertBatchToCIDs converts multiple BlockAddresses to CIDs
func (converter *AddressConverter) ConvertBatchToCIDs(addresses []*BlockAddress) []string {
	cids := make([]string, len(addresses))
	for i, address := range addresses {
		cids[i] = BlockAddressToCID(address)
	}
	return cids
}

// Migration helpers for transitioning from old IPFS client to new storage layer

// MigrationHelper assists with migrating from old IPFS client to new storage layer
type MigrationHelper struct {
	oldClient interface{}  // Old IPFS client interface
	newBackend Backend     // New storage backend
	converter  *AddressConverter
}

// NewMigrationHelper creates a new migration helper
func NewMigrationHelper(oldClient interface{}, newBackend Backend) *MigrationHelper {
	return &MigrationHelper{
		oldClient: oldClient,
		newBackend: newBackend,
		converter: NewAddressConverter(),
	}
}

// MigrateOperation demonstrates how to migrate a typical operation
func (helper *MigrationHelper) MigrateOperation(cid string) (*blocks.Block, error) {
	// Old way: direct CID usage
	// block, err := oldClient.RetrieveBlock(cid)
	
	// New way: use BlockAddress
	ctx := context.Background()
	address := helper.converter.ConvertFromLegacyCID(cid)
	return helper.newBackend.Get(ctx, address)
}

// BatchMigrateOperation demonstrates batch migration
func (helper *MigrationHelper) BatchMigrateOperation(cids []string) ([]*blocks.Block, error) {
	// Old way: loop through individual operations
	// New way: use batch operations
	ctx := context.Background()
	addresses := helper.converter.ConvertBatch(cids, BackendTypeIPFS)
	return helper.newBackend.GetMany(ctx, addresses)
}