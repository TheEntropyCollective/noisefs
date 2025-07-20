// Package config provides comprehensive configuration management for NoiseFS,
// including security presets, environment variable overrides, and validation.
//
// This package serves as the central configuration hub for the NoiseFS
// privacy-preserving distributed storage system. It provides multiple
// configuration presets optimized for different use cases, comprehensive
// validation with helpful error messages, and secure defaults.
//
// Configuration Sources (in order of precedence):
//   1. Environment variables (highest priority)
//   2. Configuration file (JSON format)
//   3. Default values (lowest priority)
//
// Security Presets:
//   - default: Balanced security and usability with all encryption enabled
//   - quickstart: Simplified configuration for new users with basic security
//   - security: Maximum privacy and security features enabled
//   - performance: Optimized for speed while maintaining essential security
//
// Key Features:
//   - Comprehensive validation with helpful error messages
//   - Environment variable overrides for all settings
//   - Security warning system for insecure configurations
//   - Multiple output formats (JSON serialization)
//   - Default configuration generation
//
// Usage Example:
//
//	// Load configuration with file and environment overrides
//	config, err := LoadConfig("/path/to/config.json")
//	if err != nil {
//		return fmt.Errorf("config error: %w", err)
//	}
//	
//	// Use security preset for maximum privacy
//	config, err := GetPresetConfig("security")
//	if err != nil {
//		return fmt.Errorf("preset error: %w", err)
//	}
//	
//	// Save configuration for future use
//	err = config.SaveToFile("/path/to/config.json")
//
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/common/logging"
)

// Config represents the complete NoiseFS system configuration with all subsystem settings.
//
// This structure contains configuration for all NoiseFS components including IPFS
// integration, caching strategies, FUSE filesystem options, logging behavior,
// performance tuning, WebUI settings, security controls, and Tor anonymization.
//
// The configuration supports multiple initialization patterns:
//   - Default configuration with secure-by-default settings
//   - Preset configurations for different use cases
//   - File-based configuration with JSON serialization
//   - Environment variable overrides for deployment flexibility
//
// Thread Safety:
//   - Config instances are safe for concurrent read access
//   - Modifications should be synchronized by the caller
//   - Validation and file operations are thread-safe
//
// Validation:
//   - All fields are validated during LoadConfig() and GetPresetConfig()
//   - Comprehensive error messages guide users to correct configurations
//   - Security warnings alert users to insecure settings
//
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
	EnableAdaptiveCache   bool `json:"enable_adaptive_cache"`
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
	BlockSize              int  `json:"block_size"`
	ReadAhead              bool `json:"read_ahead"`
	WriteBack              bool `json:"write_back"`
	MaxConcurrentOps       int  `json:"max_concurrent_ops"`
	MemoryLimit            int  `json:"memory_limit_mb"`           // Memory limit for streaming operations in MB
	StreamBufferSize       int  `json:"stream_buffer_size"`        // Buffer size for streaming pipeline
	EnableMemoryMonitoring bool `json:"enable_memory_monitoring"`  // Enable memory usage monitoring
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

// DefaultConfig returns a secure-by-default configuration suitable for most users.
//
// This configuration provides a balanced approach between security and usability,
// with all encryption features enabled and reasonable performance settings.
// It serves as the foundation for other preset configurations.
//
// Key Characteristics:
//   - All security features enabled (encryption, password protection, etc.)
//   - Conservative performance settings for stability
//   - Tor enabled for upload anonymization
//   - TLS enabled for WebUI security
//   - Structured logging for operational monitoring
//
// Security Features Enabled:
//   - Master encryption enabled
//   - Descriptor and index encryption
//   - Password protection required
//   - Secure memory handling
//   - Interactive password prompting
//
// Performance Settings:
//   - 512MB memory limit for reasonable resource usage
//   - 1000 block cache size for good performance
//   - 10 concurrent operations for stability
//   - Default 128KB block size for privacy optimization
//
// Returns:
//   A new Config instance with secure default values
//
// Complexity: O(1) - Simple structure initialization
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	defaultIndexPath := filepath.Join(homeDir, ".noisefs", "index.json")

	return &Config{
		// IPFS configuration for local node connectivity
		// Standard IPFS API endpoint with reasonable timeout for network operations
		IPFS: IPFSConfig{
			APIEndpoint: "127.0.0.1:5001", // Local IPFS node - standard port
			Timeout:     30,               // 30 seconds - balanced for normal operations
		},
		// Cache configuration balancing performance and resource usage
		// Altruistic caching enhances network contribution and anonymity
		Cache: CacheConfig{
			BlockCacheSize:      1000, // Good balance of memory usage and cache hits
			MemoryLimit:         512,  // 512MB - reasonable for most systems
			EnableAltruistic:    true, // Help network and improve anonymity
			MinPersonalCacheMB:  256,  // Reserve half for user's own files
			EnableAdaptiveCache: true, // Enable adaptive cache by default
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
			BlockSize:              blocks.DefaultBlockSize,
			ReadAhead:              false,
			WriteBack:              false,
			MaxConcurrentOps:       10,
			MemoryLimit:            512,  // 512MB default for streaming
			StreamBufferSize:       10,   // Default buffer size
			EnableMemoryMonitoring: false, // Disabled by default
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
		// Security configuration with comprehensive protection enabled
		// This represents our secure-by-default philosophy
		Security: SecurityConfig{
			// Master encryption control - disables ALL encryption if false
			EnableEncryption:   true,  // Core security requirement
			EncryptDescriptors: true,  // Protect file metadata from inspection
			DefaultEncrypted:   true,  // Encrypt all new files automatically
			RequirePassword:    true,  // Enforce access control
			PasswordPrompt:     true,  // Enable user-friendly password input
			EncryptLocalIndex:  true,  // Protect local file listings
			SecureMemory:       true,  // Prevent sensitive data swapping
			AntiForensics:      false, // Optional advanced feature
		},
		// Tor configuration prioritizing upload anonymization
		// Downloads disabled by default for better performance
		Tor: TorConfig{
			Enabled:         true,                  // Enable Tor anonymization
			SOCKSProxy:      "127.0.0.1:9050",     // Standard Tor SOCKS proxy
			ControlPort:     "127.0.0.1:9051",     // Tor control interface
			UploadEnabled:   true,                  // Anonymize uploads for privacy
			UploadJitterMin: 1,                     // Timing obfuscation minimum
			UploadJitterMax: 5,                     // Timing obfuscation maximum
			DownloadEnabled: false,                 // Prioritize speed over anonymity
			AnnounceEnabled: true,                  // Participate in anonymous announcements
		},
	}
}

// QuickStartConfig returns a simplified configuration optimized for new users.
//
// This preset prioritizes ease of use and quick setup while maintaining essential
// security features. It reduces complexity by disabling advanced features that
// may be confusing for newcomers while keeping core encryption enabled.
//
// Key Simplifications:
//   - No password requirement (but encryption still enabled)
//   - Tor disabled for faster setup and operation
//   - Smaller memory footprint for resource-constrained systems
//   - Reduced logging verbosity
//   - Conservative performance settings for stability
//
// Security Trade-offs:
//   - Local index not encrypted (for simplicity)
//   - No secure memory protection (may impact security)
//   - No anti-forensic features
//   - Reduced cache participation
//
// Best for:
//   - First-time users exploring NoiseFS
//   - Development and testing environments
//   - Systems with limited resources
//   - Users who prioritize ease of use over maximum security
//
// Returns:
//   A new Config instance optimized for quick start
//
// Complexity: O(1) - Simple structure initialization
func QuickStartConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	defaultIndexPath := filepath.Join(homeDir, ".noisefs", "index.json")

	return &Config{
		IPFS: IPFSConfig{
			APIEndpoint: "127.0.0.1:5001",
			Timeout:     30,
		},
		Cache: CacheConfig{
			BlockCacheSize:      500,   // Smaller cache for quick start
			MemoryLimit:         256,   // Conservative memory usage
			EnableAltruistic:    false, // Disabled for simplicity
			MinPersonalCacheMB:  128,
			EnableAdaptiveCache: false, // Use simple cache for quick start
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
			BlockSize:              blocks.DefaultBlockSize,
			ReadAhead:              false,
			WriteBack:              false,
			MaxConcurrentOps:       5,    // Conservative for stability
			MemoryLimit:            256,  // Conservative memory usage
			StreamBufferSize:       5,    // Smaller buffer for quick start
			EnableMemoryMonitoring: false,
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

// SecurityPresetConfig returns a configuration optimized for maximum privacy and security.
//
// This preset enables all available security features and uses conservative
// settings optimized for protecting sensitive data. Performance is secondary
// to security in this configuration.
//
// Enhanced Security Features:
//   - All encryption features enabled
//   - Anti-forensic measures activated
//   - Tor enabled for all operations (uploads and downloads)
//   - Extended timing jitter for better anonymity
//   - TLS 1.3 minimum for maximum transport security
//   - Larger cache for better anonymity set
//   - Minimal logging to reduce information leakage
//
// Performance Characteristics:
//   - Longer timeouts to accommodate Tor latency
//   - Conservative concurrency for stability
//   - Larger memory allocation for security operations
//   - JSON logging for security analysis
//
// Security Hardening:
//   - Error-level logging only (minimal information disclosure)
//   - Strict localhost binding for WebUI
//   - Maximum TLS security settings
//   - Enhanced timing obfuscation
//
// Best for:
//   - High-security environments
//   - Sensitive data storage
//   - Users prioritizing privacy over performance
//   - Adversarial network environments
//
// Returns:
//   A new Config instance with maximum security settings
//
// Complexity: O(1) - Simple structure initialization
func SecurityPresetConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	defaultIndexPath := filepath.Join(homeDir, ".noisefs", "index.json")

	return &Config{
		IPFS: IPFSConfig{
			APIEndpoint: "127.0.0.1:5001",
			Timeout:     60, // Longer timeout for Tor operations
		},
		Cache: CacheConfig{
			BlockCacheSize:      2000, // Larger cache for better anonymity
			MemoryLimit:         1024, // More memory for security operations
			EnableAltruistic:    true, // Enhanced network participation
			MinPersonalCacheMB:  512,
			EnableAdaptiveCache: true, // Enable adaptive cache for security
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

// PerformancePresetConfig returns a configuration optimized for maximum performance.
//
// This preset prioritizes speed and throughput while maintaining essential
// security features. It disables performance-impacting security features
// and enables optimizations that may reduce privacy.
//
// Performance Optimizations:
//   - Tor disabled for maximum speed
//   - Large cache sizes for better hit rates
//   - High concurrency limits
//   - Read-ahead and write-back optimizations enabled
//   - Shorter timeouts for faster failure detection
//   - Large memory allocations for caching
//
// Security Considerations:
//   - Core encryption still enabled
//   - Local index encryption disabled (performance impact)
//   - Secure memory disabled (performance impact)
//   - Anti-forensics disabled (performance impact)
//   - Minimal timing obfuscation
//
// Resource Usage:
//   - 2GB memory limit for extensive caching
//   - 5000 block cache size
//   - 50 concurrent operations
//   - Large personal cache allocation
//
// Best for:
//   - High-throughput applications
//   - Trusted network environments
//   - Users prioritizing speed over maximum privacy
//   - Development and testing scenarios
//
// Returns:
//   A new Config instance optimized for performance
//
// Complexity: O(1) - Simple structure initialization
func PerformancePresetConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	defaultIndexPath := filepath.Join(homeDir, ".noisefs", "index.json")

	return &Config{
		IPFS: IPFSConfig{
			APIEndpoint: "127.0.0.1:5001",
			Timeout:     15, // Shorter timeout for speed
		},
		Cache: CacheConfig{
			BlockCacheSize:      5000, // Large cache for performance
			MemoryLimit:         2048, // High memory allocation
			EnableAltruistic:    true,
			MinPersonalCacheMB:  1024, // Large personal cache
			EnableAdaptiveCache: true, // Enable adaptive cache for performance
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

// GetPresetConfig returns a configuration based on the specified preset name.
//
// This function provides a convenient way to access predefined configurations
// optimized for different use cases. Each preset represents a carefully balanced
// combination of security, performance, and usability settings.
//
// Available Presets:
//   - "default" or "": Balanced security and usability (recommended for most users)
//   - "quickstart": Simplified setup for new users with basic security
//   - "security": Maximum privacy and security features
//   - "performance": Optimized for speed with essential security
//
// Preset Selection Guide:
//   - Use "default" for production deployments requiring security
//   - Use "quickstart" for evaluation, development, or first-time setup
//   - Use "security" for high-value data or adversarial environments
//   - Use "performance" for trusted environments requiring high throughput
//
// Parameters:
//   preset: The name of the preset configuration to retrieve
//
// Returns:
//   *Config: A new configuration instance with preset values
//   error: An error if the preset name is not recognized
//
// Error Conditions:
//   - Unknown preset name
//   - Invalid or malformed preset name
//
// Complexity: O(1) - Simple string comparison and function dispatch
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

// LoadConfig loads configuration from file with environment variable overrides.
//
// This function implements the complete configuration loading pipeline:
// 1. Start with secure defaults
// 2. Load settings from JSON file (if provided and exists)
// 3. Apply environment variable overrides
// 4. Validate the complete configuration
// 5. Log security warnings for insecure settings
//
// Configuration Precedence (highest to lowest):
//   1. Environment variables (NOISEFS_*)
//   2. Configuration file (JSON format)
//   3. Default values
//
// File Format:
//   The configuration file must be valid JSON with the same structure
//   as the Config struct. Missing fields will use default values.
//
// Environment Variables:
//   All configuration options can be overridden using environment variables
//   with the NOISEFS_ prefix. Boolean values use "true"/"false".
//
// Error Handling:
//   - Missing files are ignored (uses defaults)
//   - Invalid JSON format returns an error
//   - Configuration validation failures return detailed error messages
//   - Security warnings are logged but don't prevent loading
//
// Parameters:
//   configPath: Path to JSON configuration file (empty string to skip file loading)
//
// Returns:
//   *Config: A fully loaded and validated configuration
//   error: An error if loading or validation fails
//
// Complexity: O(n) where n is the number of environment variables to check
func LoadConfig(configPath string) (*Config, error) {
	config := DefaultConfig()

	// Step 1: Load configuration from file if path provided
	// Missing files are silently ignored to allow default-only configurations
	if configPath != "" {
		if err := config.loadFromFile(configPath); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}

	// Step 2: Apply environment variable overrides
	// Environment variables have highest precedence
	config.applyEnvironmentOverrides()

	// Step 3: Validate the complete configuration
	// This ensures all settings are valid and provides helpful error messages
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	// Step 4: Log security warnings for user awareness
	// These are informational and don't prevent loading
	config.logSecurityWarnings()

	return config, nil
}

// loadFromFile loads configuration from a JSON file and merges with existing config.
//
// This method performs file-based configuration loading with graceful handling
// of missing files. It preserves existing configuration values for any fields
// not present in the JSON file.
//
// File Handling:
//   - Missing files are ignored (returns nil)
//   - Invalid file permissions return an error
//   - Malformed JSON returns a parsing error
//   - Valid JSON is merged with existing configuration
//
// JSON Parsing:
//   - Uses standard Go JSON unmarshaling
//   - Missing fields in JSON preserve existing values
//   - Extra fields in JSON are ignored
//   - Type mismatches return parsing errors
//
// Parameters:
//   path: Absolute or relative path to the JSON configuration file
//
// Returns:
//   error: nil on success, error details on failure
//
// Error Conditions:
//   - File read permission denied
//   - Invalid JSON syntax
//   - JSON type mismatches
//
// Complexity: O(n) where n is the size of the JSON file
func (c *Config) loadFromFile(path string) error {
	// Attempt to read the configuration file
	data, err := os.ReadFile(path)
	if err != nil {
		// Gracefully handle missing files - not an error condition
		if os.IsNotExist(err) {
			// File doesn't exist, continue with current configuration
			return nil
		}
		// Other errors (permissions, etc.) should be reported
		return err
	}

	return json.Unmarshal(data, c)
}

// applyEnvironmentOverrides applies environment variable overrides to configuration.
//
// This method implements comprehensive environment variable support for all
// configuration options. Environment variables have the highest precedence
// and can override both default values and file-based configuration.
//
// Naming Convention:
//   All environment variables use the NOISEFS_ prefix followed by the
//   configuration path in UPPER_CASE with underscores.
//
// Type Conversion:
//   - Strings: Used directly
//   - Integers: Parsed with strconv.Atoi (invalid values ignored)
//   - Booleans: "true" or "false" (case-insensitive)
//
// Error Handling:
//   - Invalid integer values are silently ignored
//   - Invalid boolean values are silently ignored
//   - This ensures environment variable errors don't break startup
//
// Supported Variables:
//   - IPFS: NOISEFS_IPFS_API, NOISEFS_IPFS_TIMEOUT
//   - Cache: NOISEFS_CACHE_SIZE, NOISEFS_MEMORY_LIMIT
//   - FUSE: NOISEFS_MOUNT_PATH, NOISEFS_VOLUME_NAME, etc.
//   - Logging: NOISEFS_LOG_LEVEL, NOISEFS_LOG_FORMAT, etc.
//   - Performance: NOISEFS_BLOCK_SIZE, NOISEFS_MAX_CONCURRENT_OPS, etc.
//   - WebUI: NOISEFS_WEBUI_HOST, NOISEFS_WEBUI_PORT, etc.
//   - Security: NOISEFS_ENCRYPT_DESCRIPTORS, NOISEFS_REQUIRE_PASSWORD, etc.
//
// Complexity: O(1) - Fixed number of environment variable checks
func (c *Config) applyEnvironmentOverrides() {
	// IPFS configuration overrides
	// Control IPFS connectivity and timeout behavior
	if val := os.Getenv("NOISEFS_IPFS_API"); val != "" {
		c.IPFS.APIEndpoint = val
	}
	if val := os.Getenv("NOISEFS_IPFS_TIMEOUT"); val != "" {
		// Parse timeout value, ignore if invalid
		if timeout, err := strconv.Atoi(val); err == nil {
			c.IPFS.Timeout = timeout
		}
	}

	// Cache configuration overrides
	// Control memory usage and cache behavior
	if val := os.Getenv("NOISEFS_CACHE_SIZE"); val != "" {
		// Parse cache size, ignore if invalid
		if size, err := strconv.Atoi(val); err == nil {
			c.Cache.BlockCacheSize = size
		}
	}
	if val := os.Getenv("NOISEFS_MEMORY_LIMIT"); val != "" {
		// Parse memory limit in MB, ignore if invalid
		if limit, err := strconv.Atoi(val); err == nil {
			c.Cache.MemoryLimit = limit
		}
	}
	if val := os.Getenv("NOISEFS_ENABLE_ADAPTIVE_CACHE"); val != "" {
		c.Cache.EnableAdaptiveCache = strings.ToLower(val) == "true"
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

	// Security configuration overrides
	// Control encryption and privacy features
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
	// Master encryption switch - affects all other encryption settings
	if val := os.Getenv("NOISEFS_ENABLE_ENCRYPTION"); val != "" {
		c.Security.EnableEncryption = strings.ToLower(val) == "true"
	}
	
	// WebUI TLS overrides
	if val := os.Getenv("NOISEFS_WEBUI_TLS_MIN_VERSION"); val != "" {
		c.WebUI.TLSMinVersion = val
	}
}

// Validate performs comprehensive configuration validation with helpful error messages.
//
// This method validates all configuration fields and provides actionable
// error messages that guide users to correct configurations. It checks
// for common misconfigurations and suggests appropriate values.
//
// Validation Categories:
//   - IPFS: API endpoint format and timeout values
//   - Cache: Memory limits and cache size relationships
//   - Logging: Valid levels, formats, and output destinations
//   - Performance: Block sizes and concurrency limits
//   - WebUI: Host security, port ranges, and TLS configuration
//   - Security: Encryption consistency and password requirements
//   - Tor: Proxy configuration and timing parameters
//
// Error Message Design:
//   - Describes what is wrong
//   - Suggests specific corrective actions
//   - Provides example values when appropriate
//   - References relevant presets for quick fixes
//
// Security Validation:
//   - Checks for security anti-patterns
//   - Validates encryption consistency
//   - Ensures TLS is properly configured
//   - Verifies file permissions for certificates
//
// Returns:
//   error: nil if configuration is valid, detailed error message otherwise
//
// Complexity: O(1) - Fixed number of validation checks
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
		logging.GetGlobalLogger().WithComponent("config").Warn("TLS is disabled for WebUI, which is insecure", map[string]interface{}{
			"security_issue": "webui_tls_disabled_preset",
			"recommendation": "Enable TLS by setting tls_enabled to true or use the 'security' preset",
		})
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

// SaveToFile saves the configuration to a JSON file with proper formatting.
//
// This method serializes the complete configuration to JSON and writes it
// to the specified file path. It creates parent directories as needed and
// uses proper JSON indentation for readability.
//
// File Operations:
//   - Creates parent directories if they don't exist
//   - Uses 0755 permissions for directories
//   - Uses 0644 permissions for the config file
//   - Overwrites existing files
//
// JSON Format:
//   - Pretty-printed with 2-space indentation
//   - All fields are included (no omitempty)
//   - Compatible with LoadConfig for round-trip operations
//
// Error Handling:
//   - Directory creation failures return detailed errors
//   - JSON serialization errors indicate which field failed
//   - File write errors include system error details
//
// Parameters:
//   path: Absolute or relative path for the output file
//
// Returns:
//   error: nil on success, detailed error message on failure
//
// Complexity: O(n) where n is the size of the configuration structure
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

// GetDefaultConfigPath returns the default configuration file path for the current user.
//
// This function constructs the standard configuration file path using the
// user's home directory and the NoiseFS configuration directory structure.
// The path follows platform conventions for user configuration files.
//
// Path Structure:
//   Unix/Linux/macOS: ~/.noisefs/config.json
//   Windows: %USERPROFILE%\.noisefs\config.json
//
// Directory Creation:
//   This function only returns the path; it does not create directories.
//   Use SaveToFile to automatically create parent directories.
//
// Error Conditions:
//   - Unable to determine user's home directory
//   - Home directory environment variables not set
//
// Returns:
//   string: The default configuration file path
//   error: An error if the home directory cannot be determined
//
// Complexity: O(1) - Simple path construction
func GetDefaultConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".noisefs", "config.json"), nil
}

// ValidateSecuritySettings performs comprehensive security validation with detailed guidance.
//
// This method validates security-related configuration settings and checks for
// common security anti-patterns. It provides actionable error messages and
// security recommendations to help users configure NoiseFS securely.
//
// Validation Areas:
//   - Encryption consistency (master switch vs individual features)
//   - Password requirements and prompting configuration
//   - Tor configuration and timing parameters
//   - Security feature interdependencies
//   - Performance vs security trade-offs
//
// Error Types:
//   - Critical errors: Configurations that compromise security
//   - Consistency errors: Conflicting security settings
//   - Configuration errors: Invalid parameter values
//
// Security Guidance:
//   - Provides specific preset recommendations
//   - Explains security implications of disabled features
//   - Suggests secure alternatives for misconfigurations
//   - Warns about security/performance trade-offs
//
// Logging Integration:
//   - Uses sanitized logging for security warnings
//   - Provides structured log data for security monitoring
//   - Includes recommendations in log messages
//
// Returns:
//   error: nil if security settings are valid, detailed error with guidance otherwise
//
// Complexity: O(1) - Fixed number of security checks
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
		logging.GetGlobalLogger().WithComponent("config").Info("Password protection is disabled", map[string]interface{}{
			"security_tip":   true,
			"recommendation": "Consider enabling require_password for better security",
		})
	}
	
	if !c.Security.EncryptLocalIndex && c.Security.EnableEncryption {
		logging.GetGlobalLogger().WithComponent("config").Info("Local index is not encrypted", map[string]interface{}{
			"security_tip":   true,
			"recommendation": "Enable encrypt_local_index to protect file metadata",
		})
	}
	
	if !c.Security.SecureMemory && c.Security.EnableEncryption {
		logging.GetGlobalLogger().WithComponent("config").Info("Secure memory is disabled", map[string]interface{}{
			"security_tip":   true,
			"recommendation": "Enable secure_memory to prevent sensitive data from being swapped to disk",
		})
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
			logging.GetGlobalLogger().WithComponent("config").Warn("Tor is enabled but both uploads and downloads are disabled", map[string]interface{}{
				"security_issue": "tor_misconfigured",
				"recommendation": "Enable upload_enabled for anonymity or disable Tor entirely",
			})
		}
	}
	
	// Check for security/performance trade-offs
	if c.Security.AntiForensics && (c.Performance.ReadAhead || c.Performance.WriteBack) {
		logging.GetGlobalLogger().WithComponent("config").Info("Anti-forensics is enabled with performance optimizations", map[string]interface{}{
			"security_tip":   true,
			"recommendation": "This may reduce anti-forensic effectiveness",
		})
	}
	
	return nil
}

// logSecurityWarnings logs security warnings through the sanitized logging system.
//
// This method analyzes the configuration for insecure settings and logs
// appropriate warnings to alert users without preventing system operation.
// All warnings are routed through the sanitized logging system to prevent
// sensitive information disclosure.
//
// Warning Categories:
//   - Critical: Settings that completely disable security (encryption off)
//   - Warning: Settings that reduce security (no password, no TLS)
//   - Info: Recommendations for enhanced security (Tor disabled)
//
// Logging Structure:
//   - Component: "config" for easy filtering
//   - Security issue type for automated monitoring
//   - Severity level for prioritization
//   - Specific recommendations for remediation
//
// Security Context:
//   - Uses sanitized logging to prevent data leakage
//   - Provides actionable recommendations
//   - Includes preset suggestions for quick fixes
//   - Structured data for security monitoring systems
//
// Integration:
//   - Called automatically during configuration loading
//   - Warnings don't prevent application startup
//   - Provides user education about security implications
//
// Complexity: O(1) - Fixed number of security checks
func (c *Config) logSecurityWarnings() {
	// Get logger with config component
	logger := logging.GetGlobalLogger().WithComponent("config")
	
	// Check for disabled security features - CRITICAL issues
	if !c.Security.EnableEncryption {
		logger.Error("Encryption is DISABLED - all data will be stored in plaintext", map[string]interface{}{
			"security_issue": "encryption_disabled",
			"severity":       "critical",
			"recommendation": "Enable encryption to protect your data",
		})
	}
	
	// WARNING level security issues
	if !c.Security.RequirePassword {
		logger.Warn("Password protection is disabled - anyone can access your files", map[string]interface{}{
			"security_issue": "password_protection_disabled",
			"severity":       "warning",
			"recommendation": "Enable password protection for access control",
		})
	}
	if !c.Security.EncryptDescriptors {
		logger.Warn("Descriptor encryption is disabled - file metadata may be exposed", map[string]interface{}{
			"security_issue": "descriptor_encryption_disabled",
			"severity":       "warning",
			"recommendation": "Enable descriptor encryption to protect file metadata",
		})
	}
	if !c.Security.EncryptLocalIndex {
		logger.Warn("Local index encryption is disabled - file listings may be exposed", map[string]interface{}{
			"security_issue": "local_index_encryption_disabled",
			"severity":       "warning",
			"recommendation": "Enable local index encryption to protect file listings",
		})
	}
	if !c.Security.SecureMemory {
		logger.Warn("Secure memory is disabled - sensitive data may be swapped to disk", map[string]interface{}{
			"security_issue": "secure_memory_disabled",
			"severity":       "warning",
			"recommendation": "Enable secure memory to prevent sensitive data from being swapped to disk",
		})
	}
	
	// Check WebUI security
	if !c.WebUI.TLSEnabled {
		logger.Warn("TLS is disabled for WebUI - connections are not encrypted", map[string]interface{}{
			"security_issue": "webui_tls_disabled",
			"severity":       "warning",
			"recommendation": "Enable TLS for encrypted WebUI connections",
		})
	}
	if c.WebUI.Host != "localhost" && c.WebUI.Host != "127.0.0.1" {
		logger.Warn("WebUI is accessible from external hosts - ensure proper network security", map[string]interface{}{
			"security_issue": "webui_external_access",
			"severity":       "warning",
			"webui_host":     c.WebUI.Host,
			"recommendation": "Restrict WebUI access to localhost or secure the network",
		})
	}
	
	// Check Tor configuration - INFO level
	if !c.Tor.Enabled {
		logger.Info("Tor is disabled - network traffic is not anonymized", map[string]interface{}{
			"security_notice": "tor_disabled",
			"severity":        "info",
			"recommendation":  "Consider enabling Tor for network traffic anonymization",
		})
	} else if !c.Tor.UploadEnabled {
		logger.Warn("Tor uploads disabled - upload patterns may reveal identity", map[string]interface{}{
			"security_issue": "tor_uploads_disabled",
			"severity":       "warning",
			"recommendation": "Enable Tor uploads to anonymize upload patterns",
		})
	}
}
