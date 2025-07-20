package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds all NoiseFS configuration
type Config struct {
	// Core service endpoints
	IPFS IPFSConfig `json:"ipfs"`
	
	// Storage and caching
	Cache CacheConfig `json:"cache"`
	
	// Filesystem interface
	FUSE FUSEConfig `json:"fuse"`
	
	// System configuration
	Logging LoggingConfig `json:"logging"`
	
	// Security settings
	Security SecurityConfig `json:"security"`
	
	// Network anonymization
	Network NetworkConfig `json:"network"`
	
	// Backward compatibility: computed performance config
	Performance PerformanceConfig `json:"-"` // Not serialized, computed on demand
}

// IPFSConfig holds IPFS connection settings
type IPFSConfig struct {
	APIEndpoint string `json:"api_endpoint"`
	Timeout     int    `json:"timeout_seconds"`
}

// CacheConfig holds cache and memory settings
type CacheConfig struct {
	BlockCacheSize        int `json:"block_cache_size"`
	MemoryLimit           int `json:"memory_limit_mb"`
	// Computed fields for backward compatibility
	EnableAltruistic      bool `json:"-"` // Computed: true if BlockCacheSize >= 1500
	MinPersonalCacheMB    int  `json:"-"` // Computed: MemoryLimit / 2
	AltruisticBandwidthMB int  `json:"-"` // Computed: MemoryLimit / 4 if altruistic
}

// FUSEConfig holds filesystem mount settings
type FUSEConfig struct {
	MountPath string `json:"mount_path"`
	IndexPath string `json:"index_path"`
	ReadOnly  bool   `json:"read_only"`
	
	// Computed fields for backward compatibility
	Debug bool `json:"-"` // Computed: true when logging level is "debug"
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level  string `json:"level"`  // debug, info, warn, error
	Output string `json:"output"` // console, file
	File   string `json:"file,omitempty"`
	Format string `json:"-"`      // Computed: always "text" for simplicity
}

// SecurityConfig holds security settings
type SecurityConfig struct {
	// Master encryption switch
	EnableEncryption bool `json:"enable_encryption"`
	
	// Password protection
	RequirePassword bool `json:"require_password"`
	
	// Computed fields for backward compatibility
	DefaultEncrypted   bool `json:"-"` // Computed: follows EnableEncryption
	PasswordPrompt     bool `json:"-"` // Computed: follows RequirePassword  
	EncryptDescriptors bool `json:"-"` // Computed: follows EnableEncryption
	EncryptLocalIndex  bool `json:"-"` // Computed: follows EnableEncryption
}

// NetworkConfig holds network and anonymization settings
type NetworkConfig struct {
	// Tor anonymization
	TorEnabled     bool   `json:"tor_enabled"`
	TorSOCKSProxy  string `json:"tor_socks_proxy"`
	
	// Performance settings
	MaxConcurrentOps int `json:"max_concurrent_ops"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	defaultIndexPath := filepath.Join(homeDir, ".noisefs", "index.json")

	config := &Config{
		IPFS: IPFSConfig{
			APIEndpoint: "127.0.0.1:5001",
			Timeout:     30,
		},
		Cache: CacheConfig{
			BlockCacheSize: 1000,
			MemoryLimit:    512,
		},
		FUSE: FUSEConfig{
			MountPath: "",
			IndexPath: defaultIndexPath,
			ReadOnly:  false,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Output: "console",
			File:   "",
		},
		Security: SecurityConfig{
			EnableEncryption: true,
			RequirePassword:  true,
		},
		Network: NetworkConfig{
			TorEnabled:       true,
			TorSOCKSProxy:    "127.0.0.1:9050",
			MaxConcurrentOps: 10,
		},
	}
	
	// Populate computed fields
	config.updateComputedFields()
	return config
}


// GetPresetConfig returns a configuration based on the specified preset name
// Available presets: "default"
func GetPresetConfig(preset string) (*Config, error) {
	switch preset {
	case "default", "":
		return DefaultConfig(), nil
	default:
		return nil, fmt.Errorf("unknown preset '%s'. Available presets: default", preset)
	}
}

// LoadConfig loads configuration from file with environment variable overrides
func LoadConfig(configPath string) (*Config, error) {
	config := DefaultConfig()

	// Load from file if it exists
	if configPath != "" {
		if err := config.loadFromFile(configPath); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}

	// Apply environment variable overrides
	config.applyEnvironmentOverrides()

	// Update computed fields after overrides
	config.updateComputedFields()

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	// Log security warnings if insecure settings are detected
	config.logSecurityWarnings()

	return config, nil
}

// LoadConfigWithMigration loads configuration and handles legacy format migration
func LoadConfigWithMigration(configPath string) (*Config, error) {
	if configPath == "" {
		return LoadConfig(configPath)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return LoadConfig("") // Use defaults
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Try to load as new format first
	config := DefaultConfig()
	if err := json.Unmarshal(data, config); err != nil {
		// If that fails, try legacy migration
		migratedConfig, migrationErr := MigrateFromLegacy(data)
		if migrationErr != nil {
			return nil, fmt.Errorf("failed to load config (tried both new and legacy formats): new format error: %w, legacy migration error: %v", err, migrationErr)
		}
		
		// Successfully migrated, save the new format
		if saveErr := migratedConfig.SaveToFile(configPath + ".migrated"); saveErr != nil {
			fmt.Fprintf(os.Stderr, "[WARNING] Failed to save migrated config: %v\n", saveErr)
		} else {
			fmt.Fprintf(os.Stderr, "[INFO] Legacy config migrated and saved to %s.migrated\n", configPath)
		}
		
		config = migratedConfig
	}

	// Apply environment variable overrides
	config.applyEnvironmentOverrides()

	// Update computed fields
	config.updateComputedFields()

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	// Log security warnings
	config.logSecurityWarnings()

	return config, nil
}

// loadFromFile loads configuration from a JSON file
func (c *Config) loadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, use defaults
			return nil
		}
		return err
	}

	return json.Unmarshal(data, c)
}

// updateComputedFields populates computed fields based on core configuration
func (c *Config) updateComputedFields() {
	// Update cache computed fields
	c.Cache.EnableAltruistic = c.Cache.BlockCacheSize >= 1500
	c.Cache.MinPersonalCacheMB = c.Cache.MemoryLimit / 2
	if c.Cache.EnableAltruistic {
		c.Cache.AltruisticBandwidthMB = c.Cache.MemoryLimit / 4
	} else {
		c.Cache.AltruisticBandwidthMB = 0
	}
	
	// Update logging computed fields
	c.Logging.Format = "text" // Always use text format in simplified config
	
	// Update security computed fields
	c.Security.DefaultEncrypted = c.Security.EnableEncryption
	c.Security.PasswordPrompt = c.Security.RequirePassword
	c.Security.EncryptDescriptors = c.Security.EnableEncryption
	c.Security.EncryptLocalIndex = c.Security.EnableEncryption
	
	// Update FUSE computed fields
	c.FUSE.Debug = (c.Logging.Level == "debug")
	
	// Update performance computed fields
	bufferSize := c.Cache.MemoryLimit / 50
	if bufferSize < 5 {
		bufferSize = 5
	}
	if bufferSize > 50 {
		bufferSize = 50
	}
	
	c.Performance = PerformanceConfig{
		BlockSize:              131072, // 128KB - NoiseFS standard
		MaxConcurrentOps:       c.Network.MaxConcurrentOps,
		MemoryLimit:            c.Cache.MemoryLimit,
		StreamBufferSize:       bufferSize,
		EnableMemoryMonitoring: c.Cache.MemoryLimit >= 1024,
		ReadAhead:              false, // Simplified - always false
		WriteBack:              false, // Simplified - always false
	}
}

// applyEnvironmentOverrides applies environment variable overrides
func (c *Config) applyEnvironmentOverrides() {
	// IPFS overrides
	if val := os.Getenv("NOISEFS_IPFS_API"); val != "" {
		c.IPFS.APIEndpoint = val
	}
	if val := os.Getenv("NOISEFS_IPFS_TIMEOUT"); val != "" {
		if timeout, err := strconv.Atoi(val); err == nil {
			c.IPFS.Timeout = timeout
		}
	}

	// Cache overrides
	if val := os.Getenv("NOISEFS_CACHE_SIZE"); val != "" {
		if size, err := strconv.Atoi(val); err == nil {
			c.Cache.BlockCacheSize = size
		}
	}
	if val := os.Getenv("NOISEFS_MEMORY_LIMIT"); val != "" {
		if limit, err := strconv.Atoi(val); err == nil {
			c.Cache.MemoryLimit = limit
		}
	}

	// FUSE overrides
	if val := os.Getenv("NOISEFS_MOUNT_PATH"); val != "" {
		c.FUSE.MountPath = val
	}
	if val := os.Getenv("NOISEFS_INDEX_PATH"); val != "" {
		c.FUSE.IndexPath = val
	}
	if val := os.Getenv("NOISEFS_READ_ONLY"); val != "" {
		c.FUSE.ReadOnly = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_DEBUG"); val != "" {
		c.FUSE.Debug = strings.ToLower(val) == "true"
	}

	// Logging overrides
	if val := os.Getenv("NOISEFS_LOG_LEVEL"); val != "" {
		c.Logging.Level = val
	}
	if val := os.Getenv("NOISEFS_LOG_OUTPUT"); val != "" {
		c.Logging.Output = val
	}
	if val := os.Getenv("NOISEFS_LOG_FILE"); val != "" {
		c.Logging.File = val
	}

	// Security overrides
	if val := os.Getenv("NOISEFS_ENABLE_ENCRYPTION"); val != "" {
		c.Security.EnableEncryption = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_DEFAULT_ENCRYPTED"); val != "" {
		c.Security.DefaultEncrypted = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_REQUIRE_PASSWORD"); val != "" {
		c.Security.RequirePassword = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_PASSWORD_PROMPT"); val != "" {
		c.Security.PasswordPrompt = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_ENCRYPT_DESCRIPTORS"); val != "" {
		c.Security.EncryptDescriptors = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_ENCRYPT_LOCAL_INDEX"); val != "" {
		c.Security.EncryptLocalIndex = strings.ToLower(val) == "true"
	}

	// Network overrides
	if val := os.Getenv("NOISEFS_TOR_ENABLED"); val != "" {
		c.Network.TorEnabled = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_TOR_SOCKS_PROXY"); val != "" {
		c.Network.TorSOCKSProxy = val
	}
	if val := os.Getenv("NOISEFS_MAX_CONCURRENT_OPS"); val != "" {
		if ops, err := strconv.Atoi(val); err == nil {
			c.Network.MaxConcurrentOps = ops
		}
	}
}

// Validate validates the configuration and provides helpful suggestions
func (c *Config) Validate() error {
	// Validate IPFS configuration
	if c.IPFS.APIEndpoint == "" {
		return fmt.Errorf("IPFS API endpoint cannot be empty. Set it to '127.0.0.1:5001' for local IPFS node")
	}
	if c.IPFS.Timeout <= 0 {
		return fmt.Errorf("IPFS timeout must be positive (current: %d). Use 30 seconds for normal use or 60 seconds for Tor", c.IPFS.Timeout)
	}
	if c.IPFS.Timeout > 300 {
		return fmt.Errorf("IPFS timeout is very high (%d seconds). Consider using 30-60 seconds", c.IPFS.Timeout)
	}

	// Validate cache configuration
	if c.Cache.BlockCacheSize <= 0 {
		return fmt.Errorf("block cache size must be positive (current: %d). Use 1000 for default or 2000+ for advanced", c.Cache.BlockCacheSize)
	}
	if c.Cache.MemoryLimit <= 0 {
		return fmt.Errorf("memory limit must be positive (current: %d MB). Use 512MB for default or 1024MB+ for advanced", c.Cache.MemoryLimit)
	}

	// Validate logging configuration
	validLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true,
	}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level '%s'. Valid options: debug, info, warn, error", c.Logging.Level)
	}

	validOutputs := map[string]bool{
		"console": true, "file": true,
	}
	if !validOutputs[c.Logging.Output] {
		return fmt.Errorf("invalid log output '%s'. Valid options: console, file", c.Logging.Output)
	}
	
	// Check if file output is configured properly
	if c.Logging.Output == "file" && c.Logging.File == "" {
		return fmt.Errorf("log file path is required when output is 'file'")
	}

	// Validate network configuration
	if c.Network.MaxConcurrentOps <= 0 {
		return fmt.Errorf("max concurrent operations must be positive (current: %d). Use 10 for default or 25+ for advanced", c.Network.MaxConcurrentOps)
	}
	if c.Network.MaxConcurrentOps > 100 {
		return fmt.Errorf("max concurrent operations is very high (%d). Consider using 10-50", c.Network.MaxConcurrentOps)
	}


	// Validate security configuration
	if !c.Security.EnableEncryption {
		return fmt.Errorf("CRITICAL: Encryption is disabled. All data will be stored in plaintext")
	}
	
	if c.Security.RequirePassword && !c.Security.PasswordPrompt {
		return fmt.Errorf("password is required but prompting is disabled. Enable password_prompt or set NOISEFS_PASSWORD environment variable")
	}

	// Validate Tor configuration
	if c.Network.TorEnabled && c.Network.TorSOCKSProxy == "" {
		return fmt.Errorf("Tor is enabled but SOCKS proxy is not configured. Set tor_socks_proxy to '127.0.0.1:9050'")
	}

	// Log security warnings
	c.logSecurityWarnings()

	return nil
}

// SaveToFile saves the configuration to a JSON file
func (c *Config) SaveToFile(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to JSON with proper formatting
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	return os.WriteFile(path, data, 0644)
}

// GetDefaultConfigPath returns the default configuration file path
func GetDefaultConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".noisefs", "config.json"), nil
}

// LegacyConfig represents the old configuration structure for migration
type LegacyConfig struct {
	IPFS        map[string]interface{} `json:"ipfs"`
	Cache       map[string]interface{} `json:"cache"`
	FUSE        map[string]interface{} `json:"fuse"`
	Logging     map[string]interface{} `json:"logging"`
	Performance map[string]interface{} `json:"performance"`
	WebUI       map[string]interface{} `json:"webui"`
	Security    map[string]interface{} `json:"security"`
	Tor         map[string]interface{} `json:"tor"`
}

// MigrateFromLegacy converts a legacy configuration to the new simplified format
func MigrateFromLegacy(legacyData []byte) (*Config, error) {
	var legacy LegacyConfig
	if err := json.Unmarshal(legacyData, &legacy); err != nil {
		return nil, fmt.Errorf("failed to parse legacy config: %w", err)
	}

	config := DefaultConfig()

	// Migrate IPFS settings
	if legacy.IPFS != nil {
		if endpoint, ok := legacy.IPFS["api_endpoint"].(string); ok && endpoint != "" {
			config.IPFS.APIEndpoint = endpoint
		}
		if timeout, ok := legacy.IPFS["timeout_seconds"].(float64); ok && timeout > 0 {
			config.IPFS.Timeout = int(timeout)
		}
	}

	// Migrate cache settings
	if legacy.Cache != nil {
		if size, ok := legacy.Cache["block_cache_size"].(float64); ok && size > 0 {
			config.Cache.BlockCacheSize = int(size)
		}
		if limit, ok := legacy.Cache["memory_limit_mb"].(float64); ok && limit > 0 {
			config.Cache.MemoryLimit = int(limit)
		}
	}

	// Migrate FUSE settings
	if legacy.FUSE != nil {
		if path, ok := legacy.FUSE["mount_path"].(string); ok {
			config.FUSE.MountPath = path
		}
		if path, ok := legacy.FUSE["index_path"].(string); ok && path != "" {
			config.FUSE.IndexPath = path
		}
		if readonly, ok := legacy.FUSE["read_only"].(bool); ok {
			config.FUSE.ReadOnly = readonly
		}
		if debug, ok := legacy.FUSE["debug"].(bool); ok {
			config.FUSE.Debug = debug
		}
	}

	// Migrate logging settings
	if legacy.Logging != nil {
		if level, ok := legacy.Logging["level"].(string); ok && level != "" {
			config.Logging.Level = level
		}
		if output, ok := legacy.Logging["output"].(string); ok && output != "" {
			// Map old values to new simplified values
			switch output {
			case "console":
				config.Logging.Output = "console"
			case "file", "both":
				config.Logging.Output = "file"
			}
		}
		if file, ok := legacy.Logging["file"].(string); ok && file != "" {
			config.Logging.File = file
		}
	}

	// Migrate security settings
	if legacy.Security != nil {
		if enabled, ok := legacy.Security["enable_encryption"].(bool); ok {
			config.Security.EnableEncryption = enabled
		}
		if encrypted, ok := legacy.Security["default_encrypted"].(bool); ok {
			config.Security.DefaultEncrypted = encrypted
		}
		if password, ok := legacy.Security["require_password"].(bool); ok {
			config.Security.RequirePassword = password
		}
		if prompt, ok := legacy.Security["password_prompt"].(bool); ok {
			config.Security.PasswordPrompt = prompt
		}
		if descriptors, ok := legacy.Security["encrypt_descriptors"].(bool); ok {
			config.Security.EncryptDescriptors = descriptors
		}
		if index, ok := legacy.Security["encrypt_local_index"].(bool); ok {
			config.Security.EncryptLocalIndex = index
		}
	}

	// Migrate network settings (from Performance, WebUI, and Tor sections)
	if legacy.Performance != nil {
		if ops, ok := legacy.Performance["max_concurrent_ops"].(float64); ok && ops > 0 {
			config.Network.MaxConcurrentOps = int(ops)
		}
	}

	// WebUI is no longer supported in simplified config

	if legacy.Tor != nil {
		if enabled, ok := legacy.Tor["enabled"].(bool); ok {
			config.Network.TorEnabled = enabled
		}
		if proxy, ok := legacy.Tor["socks_proxy"].(string); ok && proxy != "" {
			config.Network.TorSOCKSProxy = proxy
		}
	}

	return config, nil
}


// logSecurityWarnings logs warnings about insecure configuration settings
func (c *Config) logSecurityWarnings() {
	warnings := []string{}
	
	// Check for disabled security features
	if !c.Security.EnableEncryption {
		warnings = append(warnings, "CRITICAL: Encryption is DISABLED - all data will be stored in plaintext")
	}
	if !c.Security.RequirePassword {
		warnings = append(warnings, "WARNING: Password protection is disabled - anyone can access your files")
	}
	if !c.Security.EncryptDescriptors {
		warnings = append(warnings, "WARNING: Descriptor encryption is disabled - file metadata may be exposed")
	}
	if !c.Security.EncryptLocalIndex {
		warnings = append(warnings, "WARNING: Local index encryption is disabled - file listings may be exposed")
	}
	
	// Check network security
	if !c.Network.TorEnabled {
		warnings = append(warnings, "INFO: Tor is disabled - network traffic is not anonymized")
	}
	
	// Log all warnings
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "[SECURITY] %s\n", warning)
	}
}

// PerformanceConfig holds performance-related configuration for backward compatibility
type PerformanceConfig struct {
	BlockSize              int  // Computed: Always 128KB (NoiseFS standard)
	MaxConcurrentOps       int  // From Network.MaxConcurrentOps 
	MemoryLimit            int  // From Cache.MemoryLimit
	StreamBufferSize       int  // Computed: MemoryLimit/50, min 5, max 50
	EnableMemoryMonitoring bool // Computed: true if MemoryLimit >= 1024MB
	ReadAhead              bool // Computed: false (simplified)
	WriteBack              bool // Computed: false (simplified)
}
