package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// Config holds all NoiseFS configuration
type Config struct {
	// IPFS Configuration
	IPFS IPFSConfig `json:"ipfs"`

	// Cache Configuration
	Cache CacheConfig `json:"cache"`

	// FUSE Configuration
	FUSE FUSEConfig `json:"fuse"`

	// Logging Configuration
	Logging LoggingConfig `json:"logging"`

	// Performance Configuration
	Performance PerformanceConfig `json:"performance"`

	// WebUI Configuration
	WebUI WebUIConfig `json:"webui"`

	// Security Configuration
	Security SecurityConfig `json:"security"`
	
	// Tor Configuration
	Tor TorConfig `json:"tor"`
}

// IPFSConfig holds IPFS-related configuration
type IPFSConfig struct {
	APIEndpoint string `json:"api_endpoint"`
	Timeout     int    `json:"timeout_seconds"`
}

// CacheConfig holds cache-related configuration
type CacheConfig struct {
	BlockCacheSize        int  `json:"block_cache_size"`
	MemoryLimit           int  `json:"memory_limit_mb"`
	EnableAltruistic      bool `json:"enable_altruistic"`
	MinPersonalCacheMB    int  `json:"min_personal_cache_mb"`
	AltruisticBandwidthMB int  `json:"altruistic_bandwidth_mb,omitempty"`
}

// FUSEConfig holds FUSE filesystem configuration
type FUSEConfig struct {
	MountPath  string `json:"mount_path"`
	VolumeName string `json:"volume_name"`
	ReadOnly   bool   `json:"read_only"`
	AllowOther bool   `json:"allow_other"`
	Debug      bool   `json:"debug"`
	IndexPath  string `json:"index_path"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `json:"level"`
	Format string `json:"format"`
	Output string `json:"output"`
	File   string `json:"file"`
}

// PerformanceConfig holds performance-related configuration
type PerformanceConfig struct {
	BlockSize        int  `json:"block_size"`
	ReadAhead        bool `json:"read_ahead"`
	WriteBack        bool `json:"write_back"`
	MaxConcurrentOps int  `json:"max_concurrent_ops"`
}

// WebUIConfig holds web UI server configuration
type WebUIConfig struct {
	Host         string   `json:"host"`
	Port         int      `json:"port"`
	TLSEnabled   bool     `json:"tls_enabled"`
	TLSCertFile  string   `json:"tls_cert_file"`
	TLSKeyFile   string   `json:"tls_key_file"`
	TLSAutoGen   bool     `json:"tls_auto_gen"`
	TLSHostnames []string `json:"tls_hostnames"`
	TLSMinVersion string  `json:"tls_min_version"` // Minimum TLS version (e.g., "1.2", "1.3")
}

// SecurityConfig holds security-related configuration
// WARNING: Disabling security features may expose sensitive data.
// Only disable security features if you fully understand the implications.
type SecurityConfig struct {
	// EncryptDescriptors controls whether file descriptors are encrypted.
	// When true, descriptors are encrypted to prevent metadata leakage.
	EncryptDescriptors bool `json:"encrypt_descriptors"`
	
	// DefaultEncrypted determines if files are encrypted by default.
	// When true, all new files are automatically encrypted.
	DefaultEncrypted   bool `json:"default_encrypted"`
	
	// RequirePassword enforces password protection for all operations.
	// When true, a password must be provided to access any files.
	RequirePassword    bool `json:"require_password"`
	
	// PasswordPrompt enables interactive password prompting.
	// When true, the system will prompt for passwords when needed.
	PasswordPrompt     bool `json:"password_prompt"`
	
	// EncryptLocalIndex encrypts the local file index.
	// When true, the file index is encrypted to protect file metadata.
	EncryptLocalIndex  bool `json:"encrypt_local_index"`
	
	// SecureMemory enables secure memory handling to prevent swapping.
	// When true, sensitive data is locked in memory and cleared after use.
	SecureMemory       bool `json:"secure_memory"`
	
	// AntiForensics enables additional anti-forensic measures.
	// When true, additional steps are taken to make forensic analysis harder.
	AntiForensics      bool `json:"anti_forensics"`
	
	// EnableEncryption is the master switch for all encryption features.
	// When false, NO encryption is performed regardless of other settings.
	EnableEncryption   bool `json:"enable_encryption"`
}

// TorConfig holds Tor-related configuration
type TorConfig struct {
	Enabled      bool   `json:"enabled"`
	SOCKSProxy   string `json:"socks_proxy"`
	ControlPort  string `json:"control_port"`
	
	// Upload settings (default: enabled for privacy)
	UploadEnabled     bool `json:"upload_enabled"`
	UploadJitterMin   int  `json:"upload_jitter_min_seconds"`
	UploadJitterMax   int  `json:"upload_jitter_max_seconds"`
	
	// Download settings (default: disabled for performance)  
	DownloadEnabled   bool `json:"download_enabled"`
	
	// Announcement settings
	AnnounceEnabled   bool `json:"announce_enabled"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	defaultIndexPath := filepath.Join(homeDir, ".noisefs", "index.json")

	return &Config{
		IPFS: IPFSConfig{
			APIEndpoint: "127.0.0.1:5001",
			Timeout:     30,
		},
		Cache: CacheConfig{
			BlockCacheSize:     1000,
			MemoryLimit:        512,
			EnableAltruistic:   true,
			MinPersonalCacheMB: 256, // Half of default memory limit
		},
		FUSE: FUSEConfig{
			MountPath:  "",
			VolumeName: "NoiseFS",
			ReadOnly:   false,
			AllowOther: false,
			Debug:      false,
			IndexPath:  defaultIndexPath,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
			Output: "console",
			File:   "",
		},
		Performance: PerformanceConfig{
			BlockSize:        blocks.DefaultBlockSize,
			ReadAhead:        false,
			WriteBack:        false,
			MaxConcurrentOps: 10,
		},
		WebUI: WebUIConfig{
			Host:         "localhost",
			Port:         8443,
			TLSEnabled:   true,
			TLSCertFile:  "",
			TLSKeyFile:   "",
			TLSAutoGen:   true,
			TLSHostnames: []string{"localhost"},
			TLSMinVersion: "1.2", // Minimum TLS 1.2 for security
		},
		Security: SecurityConfig{
			// SECURE BY DEFAULT: All encryption features are enabled
			EnableEncryption:   true,  // Master encryption switch
			EncryptDescriptors: true,  // Encrypt file metadata
			DefaultEncrypted:   true,  // All files encrypted by default
			RequirePassword:    true,  // Password required for all operations
			PasswordPrompt:     true,  // Interactive password prompting
			EncryptLocalIndex:  true,  // Encrypt local file index
			SecureMemory:       true,  // Prevent memory swapping
			AntiForensics:      false, // Optional: user choice
		},
		Tor: TorConfig{
			Enabled:         true,
			SOCKSProxy:      "127.0.0.1:9050",
			ControlPort:     "127.0.0.1:9051",
			UploadEnabled:   true,  // ON by default for privacy
			UploadJitterMin: 1,     // 1 second minimum jitter
			UploadJitterMax: 5,     // 5 second maximum jitter
			DownloadEnabled: false, // OFF by default for performance
			AnnounceEnabled: true,  // ON by default for privacy
		},
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

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	// Log security warnings if insecure settings are detected
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
	if val := os.Getenv("NOISEFS_VOLUME_NAME"); val != "" {
		c.FUSE.VolumeName = val
	}
	if val := os.Getenv("NOISEFS_READ_ONLY"); val != "" {
		c.FUSE.ReadOnly = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_ALLOW_OTHER"); val != "" {
		c.FUSE.AllowOther = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_DEBUG"); val != "" {
		c.FUSE.Debug = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_INDEX_PATH"); val != "" {
		c.FUSE.IndexPath = val
	}

	// Logging overrides
	if val := os.Getenv("NOISEFS_LOG_LEVEL"); val != "" {
		c.Logging.Level = val
	}
	if val := os.Getenv("NOISEFS_LOG_FORMAT"); val != "" {
		c.Logging.Format = val
	}
	if val := os.Getenv("NOISEFS_LOG_OUTPUT"); val != "" {
		c.Logging.Output = val
	}
	if val := os.Getenv("NOISEFS_LOG_FILE"); val != "" {
		c.Logging.File = val
	}

	// Performance overrides
	if val := os.Getenv("NOISEFS_BLOCK_SIZE"); val != "" {
		if size, err := strconv.Atoi(val); err == nil {
			c.Performance.BlockSize = size
		}
	}
	if val := os.Getenv("NOISEFS_READ_AHEAD"); val != "" {
		c.Performance.ReadAhead = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_WRITE_BACK"); val != "" {
		c.Performance.WriteBack = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_MAX_CONCURRENT_OPS"); val != "" {
		if ops, err := strconv.Atoi(val); err == nil {
			c.Performance.MaxConcurrentOps = ops
		}
	}

	// WebUI overrides
	if val := os.Getenv("NOISEFS_WEBUI_HOST"); val != "" {
		c.WebUI.Host = val
	}
	if val := os.Getenv("NOISEFS_WEBUI_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			c.WebUI.Port = port
		}
	}
	if val := os.Getenv("NOISEFS_WEBUI_TLS_ENABLED"); val != "" {
		c.WebUI.TLSEnabled = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_WEBUI_TLS_CERT"); val != "" {
		c.WebUI.TLSCertFile = val
	}
	if val := os.Getenv("NOISEFS_WEBUI_TLS_KEY"); val != "" {
		c.WebUI.TLSKeyFile = val
	}
	if val := os.Getenv("NOISEFS_WEBUI_TLS_AUTO"); val != "" {
		c.WebUI.TLSAutoGen = strings.ToLower(val) == "true"
	}

	// Security overrides
	if val := os.Getenv("NOISEFS_ENCRYPT_DESCRIPTORS"); val != "" {
		c.Security.EncryptDescriptors = strings.ToLower(val) == "true"
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
	if val := os.Getenv("NOISEFS_ENCRYPT_LOCAL_INDEX"); val != "" {
		c.Security.EncryptLocalIndex = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_SECURE_MEMORY"); val != "" {
		c.Security.SecureMemory = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_ANTI_FORENSICS"); val != "" {
		c.Security.AntiForensics = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_ENABLE_ENCRYPTION"); val != "" {
		c.Security.EnableEncryption = strings.ToLower(val) == "true"
	}
	
	// WebUI TLS overrides
	if val := os.Getenv("NOISEFS_WEBUI_TLS_MIN_VERSION"); val != "" {
		c.WebUI.TLSMinVersion = val
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate IPFS configuration
	if c.IPFS.APIEndpoint == "" {
		return fmt.Errorf("IPFS API endpoint cannot be empty")
	}
	if c.IPFS.Timeout <= 0 {
		return fmt.Errorf("IPFS timeout must be positive")
	}

	// Validate cache configuration
	if c.Cache.BlockCacheSize <= 0 {
		return fmt.Errorf("block cache size must be positive")
	}
	if c.Cache.MemoryLimit <= 0 {
		return fmt.Errorf("memory limit must be positive")
	}

	// Validate logging configuration
	validLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true,
	}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}

	validFormats := map[string]bool{
		"text": true, "json": true,
	}
	if !validFormats[c.Logging.Format] {
		return fmt.Errorf("invalid log format: %s", c.Logging.Format)
	}

	validOutputs := map[string]bool{
		"console": true, "file": true, "both": true,
	}
	if !validOutputs[c.Logging.Output] {
		return fmt.Errorf("invalid log output: %s", c.Logging.Output)
	}

	// Validate performance configuration
	if c.Performance.BlockSize <= 0 {
		return fmt.Errorf("block size must be positive")
	}
	if c.Performance.MaxConcurrentOps <= 0 {
		return fmt.Errorf("max concurrent operations must be positive")
	}

	// Validate WebUI configuration
	if c.WebUI.Host == "" {
		return fmt.Errorf("WebUI host cannot be empty")
	}
	if c.WebUI.Port <= 0 || c.WebUI.Port > 65535 {
		return fmt.Errorf("WebUI port must be between 1 and 65535")
	}
	if c.WebUI.TLSEnabled && !c.WebUI.TLSAutoGen {
		if c.WebUI.TLSCertFile == "" || c.WebUI.TLSKeyFile == "" {
			return fmt.Errorf("TLS cert and key files required when TLS enabled and auto-generation disabled")
		}
	}
	
	// Validate TLS configuration
	if c.WebUI.TLSEnabled {
		validTLSVersions := map[string]bool{
			"1.0": true, "1.1": true, "1.2": true, "1.3": true,
		}
		if !validTLSVersions[c.WebUI.TLSMinVersion] {
			return fmt.Errorf("invalid TLS minimum version: %s", c.WebUI.TLSMinVersion)
		}
		// Warn about insecure TLS versions
		if c.WebUI.TLSMinVersion == "1.0" || c.WebUI.TLSMinVersion == "1.1" {
			return fmt.Errorf("TLS versions 1.0 and 1.1 are insecure and not allowed")
		}
	}
	
	// Validate security configuration
	if err := c.ValidateSecuritySettings(); err != nil {
		return fmt.Errorf("security validation failed: %w", err)
	}

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

// ValidateSecuritySettings performs comprehensive security validation
func (c *Config) ValidateSecuritySettings() error {
	// If encryption is disabled, warn about all the implications
	if !c.Security.EnableEncryption {
		// This is a critical security issue - should we allow it at all?
		if c.Security.RequirePassword {
			return fmt.Errorf("cannot require password when encryption is disabled")
		}
		if c.Security.EncryptDescriptors || c.Security.EncryptLocalIndex {
			return fmt.Errorf("cannot encrypt descriptors or index when encryption is disabled")
		}
	}
	
	// If password is required, ensure password prompting is enabled
	if c.Security.RequirePassword && !c.Security.PasswordPrompt {
		// This might be okay if password is provided via environment or API
		// But we should warn about it
	}
	
	// Validate Tor configuration if enabled
	if c.Tor.Enabled {
		if c.Tor.UploadJitterMin < 0 || c.Tor.UploadJitterMax < 0 {
			return fmt.Errorf("Tor jitter values must be non-negative")
		}
		if c.Tor.UploadJitterMin > c.Tor.UploadJitterMax {
			return fmt.Errorf("Tor upload jitter min must be <= max")
		}
	}
	
	return nil
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
	if !c.Security.SecureMemory {
		warnings = append(warnings, "WARNING: Secure memory is disabled - sensitive data may be swapped to disk")
	}
	
	// Check WebUI security
	if !c.WebUI.TLSEnabled {
		warnings = append(warnings, "WARNING: TLS is disabled for WebUI - connections are not encrypted")
	}
	if c.WebUI.Host != "localhost" && c.WebUI.Host != "127.0.0.1" {
		warnings = append(warnings, "WARNING: WebUI is accessible from external hosts - ensure proper network security")
	}
	
	// Check Tor configuration
	if !c.Tor.Enabled {
		warnings = append(warnings, "INFO: Tor is disabled - network traffic is not anonymized")
	} else if !c.Tor.UploadEnabled {
		warnings = append(warnings, "WARNING: Tor uploads disabled - upload patterns may reveal identity")
	}
	
	// Log all warnings
	for _, warning := range warnings {
		// In a real implementation, this would use the actual logger
		// For now, we'll just print to stderr
		fmt.Fprintf(os.Stderr, "[SECURITY] %s\n", warning)
	}
}
