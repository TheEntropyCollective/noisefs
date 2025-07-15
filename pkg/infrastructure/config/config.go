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

// QuickStartConfig returns a simplified configuration for new users
// Optimized for ease of use with reasonable security defaults
func QuickStartConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	defaultIndexPath := filepath.Join(homeDir, ".noisefs", "index.json")

	return &Config{
		IPFS: IPFSConfig{
			APIEndpoint: "127.0.0.1:5001",
			Timeout:     30,
		},
		Cache: CacheConfig{
			BlockCacheSize:     500,  // Smaller cache for quick start
			MemoryLimit:        256,  // Conservative memory usage
			EnableAltruistic:   false, // Disabled for simplicity
			MinPersonalCacheMB: 128,
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
			Level:  "warn", // Less verbose for new users
			Format: "text",
			Output: "console",
			File:   "",
		},
		Performance: PerformanceConfig{
			BlockSize:        blocks.DefaultBlockSize,
			ReadAhead:        false,
			WriteBack:        false,
			MaxConcurrentOps: 5, // Conservative for stability
		},
		WebUI: WebUIConfig{
			Host:         "localhost",
			Port:         8443,
			TLSEnabled:   true,
			TLSCertFile:  "",
			TLSKeyFile:   "",
			TLSAutoGen:   true,
			TLSHostnames: []string{"localhost"},
			TLSMinVersion: "1.2",
		},
		Security: SecurityConfig{
			// Simplified security - still secure but less complex
			EnableEncryption:   true,
			EncryptDescriptors: true,
			DefaultEncrypted:   true,
			RequirePassword:    false, // Simplified for quick start
			PasswordPrompt:     true,
			EncryptLocalIndex:  false, // Simplified for quick start
			SecureMemory:       false, // Simplified for quick start
			AntiForensics:      false,
		},
		Tor: TorConfig{
			Enabled:         false, // Disabled for simplicity and speed
			SOCKSProxy:      "127.0.0.1:9050",
			ControlPort:     "127.0.0.1:9051",
			UploadEnabled:   false,
			UploadJitterMin: 1,
			UploadJitterMax: 5,
			DownloadEnabled: false,
			AnnounceEnabled: false,
		},
	}
}

// SecurityConfig returns a configuration optimized for maximum privacy and security
// All security features enabled with conservative performance settings
func SecurityPresetConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	defaultIndexPath := filepath.Join(homeDir, ".noisefs", "index.json")

	return &Config{
		IPFS: IPFSConfig{
			APIEndpoint: "127.0.0.1:5001",
			Timeout:     60, // Longer timeout for Tor operations
		},
		Cache: CacheConfig{
			BlockCacheSize:     2000, // Larger cache for better anonymity
			MemoryLimit:        1024, // More memory for security operations
			EnableAltruistic:   true,  // Enhanced network participation
			MinPersonalCacheMB: 512,
		},
		FUSE: FUSEConfig{
			MountPath:  "",
			VolumeName: "NoiseFS",
			ReadOnly:   false,
			AllowOther: false,
			Debug:      false, // No debug logging for security
			IndexPath:  defaultIndexPath,
		},
		Logging: LoggingConfig{
			Level:  "error", // Minimal logging for security
			Format: "json",  // Structured logging for security analysis
			Output: "console",
			File:   "",
		},
		Performance: PerformanceConfig{
			BlockSize:        blocks.DefaultBlockSize,
			ReadAhead:        false, // Conservative for security
			WriteBack:        false, // Conservative for security
			MaxConcurrentOps: 5,     // Conservative for stability
		},
		WebUI: WebUIConfig{
			Host:         "127.0.0.1", // Strict localhost only
			Port:         8443,
			TLSEnabled:   true,
			TLSCertFile:  "",
			TLSKeyFile:   "",
			TLSAutoGen:   true,
			TLSHostnames: []string{"localhost", "127.0.0.1"},
			TLSMinVersion: "1.3", // Maximum TLS security
		},
		Security: SecurityConfig{
			// MAXIMUM SECURITY: All features enabled
			EnableEncryption:   true,
			EncryptDescriptors: true,
			DefaultEncrypted:   true,
			RequirePassword:    true,
			PasswordPrompt:     true,
			EncryptLocalIndex:  true,
			SecureMemory:       true,
			AntiForensics:      true, // Enable all anti-forensic features
		},
		Tor: TorConfig{
			Enabled:         true,
			SOCKSProxy:      "127.0.0.1:9050",
			ControlPort:     "127.0.0.1:9051",
			UploadEnabled:   true,
			UploadJitterMin: 5,  // Longer jitter for better anonymity
			UploadJitterMax: 15, // Longer jitter for better anonymity
			DownloadEnabled: true, // Enable for maximum privacy
			AnnounceEnabled: true,
		},
	}
}

// PerformanceConfig returns a configuration optimized for maximum performance
// Security features balanced with performance requirements
func PerformancePresetConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	defaultIndexPath := filepath.Join(homeDir, ".noisefs", "index.json")

	return &Config{
		IPFS: IPFSConfig{
			APIEndpoint: "127.0.0.1:5001",
			Timeout:     15, // Shorter timeout for speed
		},
		Cache: CacheConfig{
			BlockCacheSize:     5000, // Large cache for performance
			MemoryLimit:        2048, // High memory allocation
			EnableAltruistic:   true,
			MinPersonalCacheMB: 1024, // Large personal cache
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
			Level:  "warn", // Minimal logging overhead
			Format: "text",
			Output: "console",
			File:   "",
		},
		Performance: PerformanceConfig{
			BlockSize:        blocks.DefaultBlockSize,
			ReadAhead:        true,  // Enable for performance
			WriteBack:        true,  // Enable for performance
			MaxConcurrentOps: 50,    // High concurrency
		},
		WebUI: WebUIConfig{
			Host:         "localhost",
			Port:         8443,
			TLSEnabled:   true,
			TLSCertFile:  "",
			TLSKeyFile:   "",
			TLSAutoGen:   true,
			TLSHostnames: []string{"localhost"},
			TLSMinVersion: "1.2", // Balanced security/performance
		},
		Security: SecurityConfig{
			// Balanced security for performance
			EnableEncryption:   true,
			EncryptDescriptors: true,
			DefaultEncrypted:   true,
			RequirePassword:    true,
			PasswordPrompt:     true,
			EncryptLocalIndex:  false, // Disabled for performance
			SecureMemory:       false, // Disabled for performance
			AntiForensics:      false, // Disabled for performance
		},
		Tor: TorConfig{
			Enabled:         false, // Disabled for maximum performance
			SOCKSProxy:      "127.0.0.1:9050",
			ControlPort:     "127.0.0.1:9051",
			UploadEnabled:   false,
			UploadJitterMin: 1,
			UploadJitterMax: 3, // Minimal jitter for speed
			DownloadEnabled: false,
			AnnounceEnabled: false,
		},
	}
}

// GetPresetConfig returns a configuration based on the specified preset name
// Available presets: "default", "quickstart", "security", "performance"
func GetPresetConfig(preset string) (*Config, error) {
	switch preset {
	case "default", "":
		return DefaultConfig(), nil
	case "quickstart":
		return QuickStartConfig(), nil
	case "security":
		return SecurityPresetConfig(), nil
	case "performance":
		return PerformancePresetConfig(), nil
	default:
		return nil, fmt.Errorf("unknown preset '%s'. Available presets: default, quickstart, security, performance", preset)
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

// Validate validates the configuration and provides helpful suggestions
func (c *Config) Validate() error {
	// Validate IPFS configuration
	if c.IPFS.APIEndpoint == "" {
		return fmt.Errorf("IPFS API endpoint cannot be empty. Set it to '127.0.0.1:5001' for local IPFS node or use a preset: 'quickstart', 'security', or 'performance'")
	}
	if c.IPFS.Timeout <= 0 {
		return fmt.Errorf("IPFS timeout must be positive (current: %d). Try setting it to 30 seconds for normal use, 60 seconds for Tor connections, or 15 seconds for performance optimization", c.IPFS.Timeout)
	}
	if c.IPFS.Timeout > 300 {
		return fmt.Errorf("IPFS timeout is very high (%d seconds). Consider using a shorter timeout (15-60 seconds) to improve responsiveness", c.IPFS.Timeout)
	}

	// Validate cache configuration
	if c.Cache.BlockCacheSize <= 0 {
		return fmt.Errorf("block cache size must be positive (current: %d). Recommended values: 500 for quick start, 1000 for normal use, 2000+ for security/performance", c.Cache.BlockCacheSize)
	}
	if c.Cache.MemoryLimit <= 0 {
		return fmt.Errorf("memory limit must be positive (current: %d MB). Recommended values: 256MB for quick start, 512MB for normal use, 1024MB+ for performance", c.Cache.MemoryLimit)
	}
	if c.Cache.MinPersonalCacheMB > c.Cache.MemoryLimit {
		return fmt.Errorf("personal cache minimum (%d MB) cannot exceed total memory limit (%d MB). Set personal cache to at most %d MB", 
			c.Cache.MinPersonalCacheMB, c.Cache.MemoryLimit, c.Cache.MemoryLimit/2)
	}

	// Validate logging configuration
	validLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true,
	}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level '%s'. Valid options: debug, info, warn, error. Use 'info' for normal operation, 'warn' for minimal output, or 'debug' for troubleshooting", c.Logging.Level)
	}

	validFormats := map[string]bool{
		"text": true, "json": true,
	}
	if !validFormats[c.Logging.Format] {
		return fmt.Errorf("invalid log format '%s'. Valid options: text, json. Use 'text' for human-readable logs or 'json' for automated processing", c.Logging.Format)
	}

	validOutputs := map[string]bool{
		"console": true, "file": true, "both": true,
	}
	if !validOutputs[c.Logging.Output] {
		return fmt.Errorf("invalid log output '%s'. Valid options: console, file, both. Use 'console' for development, 'file' for production, or 'both' for comprehensive logging", c.Logging.Output)
	}
	
	// Check if file output is configured properly
	if (c.Logging.Output == "file" || c.Logging.Output == "both") && c.Logging.File == "" {
		return fmt.Errorf("log file path is required when output is set to '%s'. Set logging.file to a valid file path like '/var/log/noisefs.log'", c.Logging.Output)
	}

	// Validate performance configuration
	if c.Performance.BlockSize <= 0 {
		return fmt.Errorf("block size must be positive (current: %d). Use the default block size or a power of 2 value (e.g., 32768, 65536, 131072)", c.Performance.BlockSize)
	}
	if c.Performance.BlockSize < 1024 {
		return fmt.Errorf("block size is very small (%d bytes), which may impact performance. Consider using at least 32KB for better efficiency", c.Performance.BlockSize)
	}
	if c.Performance.MaxConcurrentOps <= 0 {
		return fmt.Errorf("max concurrent operations must be positive (current: %d). Recommended values: 5 for stability, 10 for normal use, 50+ for high performance", c.Performance.MaxConcurrentOps)
	}
	if c.Performance.MaxConcurrentOps > 100 {
		return fmt.Errorf("max concurrent operations is very high (%d), which may overwhelm system resources. Consider using 10-50 for most use cases", c.Performance.MaxConcurrentOps)
	}

	// Validate WebUI configuration
	if c.WebUI.Host == "" {
		return fmt.Errorf("WebUI host cannot be empty. Use 'localhost' for local access only, '127.0.0.1' for strict localhost, or '0.0.0.0' for external access (not recommended)")
	}
	if c.WebUI.Port <= 0 || c.WebUI.Port > 65535 {
		return fmt.Errorf("WebUI port must be between 1 and 65535 (current: %d). Recommended ports: 8443 (default), 8080, or 3000", c.WebUI.Port)
	}
	if c.WebUI.Port < 1024 && c.WebUI.Port != 80 && c.WebUI.Port != 443 {
		return fmt.Errorf("WebUI port %d is a privileged port. Use ports above 1024 (e.g., 8443, 8080) or run as administrator", c.WebUI.Port)
	}
	if c.WebUI.TLSEnabled && !c.WebUI.TLSAutoGen {
		if c.WebUI.TLSCertFile == "" || c.WebUI.TLSKeyFile == "" {
			return fmt.Errorf("TLS cert and key files required when TLS enabled and auto-generation disabled. Provide valid paths or set tls_auto_gen to true")
		}
		// Check if files exist
		if _, err := os.Stat(c.WebUI.TLSCertFile); os.IsNotExist(err) {
			return fmt.Errorf("TLS certificate file does not exist: %s. Generate certificates or enable auto-generation", c.WebUI.TLSCertFile)
		}
		if _, err := os.Stat(c.WebUI.TLSKeyFile); os.IsNotExist(err) {
			return fmt.Errorf("TLS key file does not exist: %s. Generate certificates or enable auto-generation", c.WebUI.TLSKeyFile)
		}
	}
	
	// Validate TLS configuration
	if c.WebUI.TLSEnabled {
		validTLSVersions := map[string]bool{
			"1.0": true, "1.1": true, "1.2": true, "1.3": true,
		}
		if !validTLSVersions[c.WebUI.TLSMinVersion] {
			return fmt.Errorf("invalid TLS minimum version '%s'. Valid options: 1.2, 1.3. Use '1.2' for compatibility or '1.3' for maximum security", c.WebUI.TLSMinVersion)
		}
		// Warn about insecure TLS versions
		if c.WebUI.TLSMinVersion == "1.0" || c.WebUI.TLSMinVersion == "1.1" {
			return fmt.Errorf("TLS versions 1.0 and 1.1 are insecure and not allowed. Use TLS 1.2 or 1.3 for security")
		}
	} else {
		// Warning for disabled TLS, but don't make it an error to allow QuickStart preset
		fmt.Fprintf(os.Stderr, "[SECURITY WARNING] TLS is disabled for WebUI, which is insecure. Enable TLS by setting tls_enabled to true or use the 'security' preset\n")
	}
	
	// Validate host security
	if c.WebUI.Host != "localhost" && c.WebUI.Host != "127.0.0.1" && !c.WebUI.TLSEnabled {
		return fmt.Errorf("WebUI is accessible from external hosts (%s) without TLS encryption. Enable TLS or restrict host to 'localhost' for security", c.WebUI.Host)
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

// ValidateSecuritySettings performs comprehensive security validation with helpful guidance
func (c *Config) ValidateSecuritySettings() error {
	// If encryption is disabled, provide clear guidance
	if !c.Security.EnableEncryption {
		if c.Security.RequirePassword {
			return fmt.Errorf("cannot require password when encryption is disabled. Either enable encryption or disable password requirement. Use 'security' preset for maximum protection")
		}
		if c.Security.EncryptDescriptors || c.Security.EncryptLocalIndex {
			return fmt.Errorf("cannot encrypt descriptors or index when encryption is disabled. Enable master encryption (enable_encryption: true) or disable specific encryption features")
		}
		// Warn user about security implications
		return fmt.Errorf("CRITICAL SECURITY WARNING: Encryption is completely disabled. All data will be stored in plaintext. Use 'security' or 'quickstart' preset for secure operation")
	}
	
	// Validate password configuration
	if c.Security.RequirePassword && !c.Security.PasswordPrompt {
		return fmt.Errorf("password is required but prompting is disabled. Enable password_prompt or provide password via environment variable NOISEFS_PASSWORD")
	}
	
	// Provide security best practice guidance
	if !c.Security.RequirePassword && c.Security.EnableEncryption {
		// This is a warning, not an error - encryption without password is still valid
		fmt.Fprintf(os.Stderr, "[SECURITY TIP] Password protection is disabled. Consider enabling require_password for better security\n")
	}
	
	if !c.Security.EncryptLocalIndex && c.Security.EnableEncryption {
		fmt.Fprintf(os.Stderr, "[SECURITY TIP] Local index is not encrypted. Enable encrypt_local_index to protect file metadata\n")
	}
	
	if !c.Security.SecureMemory && c.Security.EnableEncryption {
		fmt.Fprintf(os.Stderr, "[SECURITY TIP] Secure memory is disabled. Enable secure_memory to prevent sensitive data from being swapped to disk\n")
	}
	
	// Validate Tor configuration if enabled
	if c.Tor.Enabled {
		if c.Tor.UploadJitterMin < 0 || c.Tor.UploadJitterMax < 0 {
			return fmt.Errorf("Tor jitter values must be non-negative (upload_jitter_min: %d, upload_jitter_max: %d). Use positive values like 1-5 seconds for basic anonymity", 
				c.Tor.UploadJitterMin, c.Tor.UploadJitterMax)
		}
		if c.Tor.UploadJitterMin > c.Tor.UploadJitterMax {
			return fmt.Errorf("Tor upload jitter min (%d) must be <= max (%d). Try setting min to 1 and max to 5 for basic timing obfuscation", 
				c.Tor.UploadJitterMin, c.Tor.UploadJitterMax)
		}
		if c.Tor.UploadJitterMax > 60 {
			return fmt.Errorf("Tor upload jitter max is very high (%d seconds), which may impact usability. Consider using 5-15 seconds for good anonymity without excessive delays", 
				c.Tor.UploadJitterMax)
		}
		
		// Check Tor proxy accessibility
		if c.Tor.SOCKSProxy == "" {
			return fmt.Errorf("Tor is enabled but SOCKS proxy is not configured. Set socks_proxy to '127.0.0.1:9050' for standard Tor setup")
		}
		
		// Provide Tor usage guidance
		if !c.Tor.UploadEnabled && !c.Tor.DownloadEnabled {
			fmt.Fprintf(os.Stderr, "[SECURITY WARNING] Tor is enabled but both uploads and downloads are disabled. Enable upload_enabled for anonymity or disable Tor entirely\n")
		}
	}
	
	// Check for security/performance trade-offs
	if c.Security.AntiForensics && (c.Performance.ReadAhead || c.Performance.WriteBack) {
		fmt.Fprintf(os.Stderr, "[SECURITY TIP] Anti-forensics is enabled with performance optimizations. This may reduce anti-forensic effectiveness\n")
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
