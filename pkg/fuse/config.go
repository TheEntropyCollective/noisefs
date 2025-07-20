package fuse

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// FuseConfig contains all configurable parameters for the FUSE filesystem
type FuseConfig struct {
	// Cache configurations
	Cache CacheConfig `json:"cache"`
	
	// Security settings
	Security SecurityConfig `json:"security"`
	
	// Performance tuning
	Performance PerformanceConfig `json:"performance"`
	
	// Mount options and FUSE-specific settings
	Mount MountConfig `json:"mount"`
	
	// Index management settings
	Index IndexConfig `json:"index"`
}

// CacheConfig holds cache-related configuration
type CacheConfig struct {
	// Directory cache settings
	DirectoryMaxSize        int           `json:"directory_max_size"`        // Maximum number of cached manifests
	DirectoryTTL            time.Duration `json:"directory_ttl"`             // Time to live for cache entries
	DirectoryEnableMetrics  bool          `json:"directory_enable_metrics"`  // Enable cache metrics
	
	// Manifest size estimation parameters
	ManifestEntryOverhead   int           `json:"manifest_entry_overhead"`   // Bytes per manifest entry (default: 100)
	ManifestBaseOverhead    int           `json:"manifest_base_overhead"`    // Base overhead per manifest (default: 1024)
	
	// Cache warming settings
	WarmCacheMaxDirs        int           `json:"warm_cache_max_dirs"`       // Max directories to warm on startup (default: 10)
}

// SecurityConfig holds security-related configuration
type SecurityConfig struct {
	// Encryption settings
	EnableEncryption        bool          `json:"enable_encryption"`         // Enable encrypted index
	SecureMemoryLocking     bool          `json:"secure_memory_locking"`     // Lock sensitive memory pages
	SecureDeletion          bool          `json:"secure_deletion"`           // Secure file deletion with overwrite
	SecureDeletionPasses    int           `json:"secure_deletion_passes"`    // Number of overwrite passes (default: 3)
	
	// File permissions
	DefaultFileMode         os.FileMode   `json:"default_file_mode"`         // Default file permissions (default: 0644)
	DefaultDirMode          os.FileMode   `json:"default_dir_mode"`          // Default directory permissions (default: 0755)
	IndexFileMode           os.FileMode   `json:"index_file_mode"`           // Index file permissions (default: 0600)
	IndexDirMode            os.FileMode   `json:"index_dir_mode"`            // Index directory permissions (default: 0700)
	MountDirMode            os.FileMode   `json:"mount_dir_mode"`            // Mount directory permissions (default: 0755)
}

// PerformanceConfig holds performance-related configuration
type PerformanceConfig struct {
	// Concurrent operations
	MaxConcurrentOperations int           `json:"max_concurrent_operations"` // Max concurrent file operations
	
	// Buffer sizes
	ReadBufferSize          int           `json:"read_buffer_size"`          // Read buffer size in bytes
	WriteBufferSize         int           `json:"write_buffer_size"`         // Write buffer size in bytes
	
	// Timeouts
	OperationTimeout        time.Duration `json:"operation_timeout"`         // Timeout for file operations
	MountTimeout            time.Duration `json:"mount_timeout"`             // Timeout for mount operations
}

// MountConfig holds FUSE mount-specific configuration
type MountConfig struct {
	// FUSE options
	AllowOther              bool          `json:"allow_other"`               // Allow other users to access
	Debug                   bool          `json:"debug"`                     // Enable FUSE debug output
	ReadOnly                bool          `json:"read_only"`                 // Mount as read-only
	
	// Mount behavior
	AutoUnmountOnExit       bool          `json:"auto_unmount_on_exit"`      // Auto-unmount on process exit
	EnableXAttr             bool          `json:"enable_xattr"`              // Enable extended attributes
	EnableSymlinks          bool          `json:"enable_symlinks"`           // Enable symbolic links (not yet supported)
	EnableHardlinks         bool          `json:"enable_hardlinks"`          // Enable hard links
	
	// Volume settings
	DefaultVolumeName       string        `json:"default_volume_name"`       // Default volume name
	FilesSubdirectory       string        `json:"files_subdirectory"`        // Name of files subdirectory (default: "files")
}

// IndexConfig holds index management configuration
type IndexConfig struct {
	// Auto-save behavior
	AutoSave                bool          `json:"auto_save"`                 // Auto-save index on changes
	SaveInterval            time.Duration `json:"save_interval"`             // Interval for periodic saves
	
	// Index file settings
	BackupOnMigration       bool          `json:"backup_on_migration"`       // Backup old index when migrating
	CompactOnSave           bool          `json:"compact_on_save"`           // Compact index when saving
	
	// Version and compatibility
	Version                 string        `json:"version"`                   // Index format version
	EncryptedVersion        string        `json:"encrypted_version"`         // Encrypted index format version
}

// DefaultFuseConfig returns the default configuration for standard usage
func DefaultFuseConfig() *FuseConfig {
	return &FuseConfig{
		Cache: CacheConfig{
			DirectoryMaxSize:       100,
			DirectoryTTL:          30 * time.Minute,
			DirectoryEnableMetrics: true,
			ManifestEntryOverhead:  100,
			ManifestBaseOverhead:   1024,
			WarmCacheMaxDirs:       10,
		},
		Security: SecurityConfig{
			EnableEncryption:       false,
			SecureMemoryLocking:    false,
			SecureDeletion:         false,
			SecureDeletionPasses:   3,
			DefaultFileMode:        0644,
			DefaultDirMode:         0755,
			IndexFileMode:          0600,
			IndexDirMode:           0700,
			MountDirMode:           0755,
		},
		Performance: PerformanceConfig{
			MaxConcurrentOperations: 10,
			ReadBufferSize:         64 * 1024,  // 64KB
			WriteBufferSize:        64 * 1024,  // 64KB
			OperationTimeout:       30 * time.Second,
			MountTimeout:           10 * time.Second,
		},
		Mount: MountConfig{
			AllowOther:            false,
			Debug:                 false,
			ReadOnly:              false,
			AutoUnmountOnExit:     true,
			EnableXAttr:           true,
			EnableSymlinks:        false, // Not yet supported
			EnableHardlinks:       true,
			DefaultVolumeName:     "noisefs",
			FilesSubdirectory:     "files",
		},
		Index: IndexConfig{
			AutoSave:              true,
			SaveInterval:          5 * time.Minute,
			BackupOnMigration:     true,
			CompactOnSave:         false,
			Version:               "1.0",
			EncryptedVersion:      "1.0-encrypted",
		},
	}
}

// PerformanceFuseConfig returns configuration optimized for high-performance scenarios
func PerformanceFuseConfig() *FuseConfig {
	config := DefaultFuseConfig()
	
	// Optimize cache settings for performance
	config.Cache.DirectoryMaxSize = 500
	config.Cache.DirectoryTTL = 60 * time.Minute
	config.Cache.WarmCacheMaxDirs = 50
	
	// Increase buffer sizes and concurrent operations
	config.Performance.MaxConcurrentOperations = 50
	config.Performance.ReadBufferSize = 256 * 1024   // 256KB
	config.Performance.WriteBufferSize = 256 * 1024  // 256KB
	config.Performance.OperationTimeout = 60 * time.Second
	
	// Optimize index settings
	config.Index.SaveInterval = 15 * time.Minute // Save less frequently
	config.Index.CompactOnSave = true            // Keep index compact
	
	return config
}

// SecureFuseConfig returns configuration optimized for maximum security
func SecureFuseConfig() *FuseConfig {
	config := DefaultFuseConfig()
	
	// Enable all security features
	config.Security.EnableEncryption = true
	config.Security.SecureMemoryLocking = true
	config.Security.SecureDeletion = true
	config.Security.SecureDeletionPasses = 5
	
	// Stricter file permissions
	config.Security.DefaultFileMode = 0600
	config.Security.DefaultDirMode = 0700
	config.Security.IndexFileMode = 0600
	config.Security.IndexDirMode = 0700
	config.Security.MountDirMode = 0700
	
	// Security-focused mount options
	config.Mount.AllowOther = false
	config.Mount.ReadOnly = true  // Default to read-only for maximum security
	config.Mount.EnableXAttr = false  // Disable extended attributes for security
	config.Mount.EnableHardlinks = false  // Disable hard links for security
	
	// More frequent saves for data integrity
	config.Index.SaveInterval = 1 * time.Minute
	config.Index.BackupOnMigration = true
	
	// Smaller cache for security (less data in memory)
	config.Cache.DirectoryMaxSize = 50
	config.Cache.DirectoryTTL = 15 * time.Minute
	config.Cache.WarmCacheMaxDirs = 5
	
	return config
}

// ValidateConfig validates the configuration and returns an error if invalid
func ValidateConfig(config *FuseConfig) error {
	// Validate cache settings
	if config.Cache.DirectoryMaxSize <= 0 {
		return fmt.Errorf("cache directory_max_size must be positive")
	}
	if config.Cache.DirectoryTTL <= 0 {
		return fmt.Errorf("cache directory_ttl must be positive")
	}
	if config.Cache.ManifestEntryOverhead <= 0 {
		return fmt.Errorf("cache manifest_entry_overhead must be positive")
	}
	if config.Cache.ManifestBaseOverhead <= 0 {
		return fmt.Errorf("cache manifest_base_overhead must be positive")
	}
	if config.Cache.WarmCacheMaxDirs < 0 {
		return fmt.Errorf("cache warm_cache_max_dirs must be non-negative")
	}
	
	// Validate security settings
	if config.Security.SecureDeletionPasses <= 0 {
		return fmt.Errorf("security secure_deletion_passes must be positive")
	}
	
	// Validate performance settings
	if config.Performance.MaxConcurrentOperations <= 0 {
		return fmt.Errorf("performance max_concurrent_operations must be positive")
	}
	if config.Performance.ReadBufferSize <= 0 {
		return fmt.Errorf("performance read_buffer_size must be positive")
	}
	if config.Performance.WriteBufferSize <= 0 {
		return fmt.Errorf("performance write_buffer_size must be positive")
	}
	if config.Performance.OperationTimeout <= 0 {
		return fmt.Errorf("performance operation_timeout must be positive")
	}
	if config.Performance.MountTimeout <= 0 {
		return fmt.Errorf("performance mount_timeout must be positive")
	}
	
	// Validate mount settings
	if config.Mount.FilesSubdirectory == "" {
		return fmt.Errorf("mount files_subdirectory cannot be empty")
	}
	
	// Validate index settings
	if config.Index.SaveInterval <= 0 {
		return fmt.Errorf("index save_interval must be positive")
	}
	if config.Index.Version == "" {
		return fmt.Errorf("index version cannot be empty")
	}
	if config.Index.EncryptedVersion == "" {
		return fmt.Errorf("index encrypted_version cannot be empty")
	}
	
	return nil
}

// LoadConfigFromFile loads configuration from a JSON file
func LoadConfigFromFile(configPath string) (*FuseConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	
	config := DefaultFuseConfig()
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	
	if err := ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	return config, nil
}

// SaveConfigToFile saves configuration to a JSON file
func SaveConfigToFile(config *FuseConfig, configPath string) error {
	if err := ValidateConfig(config); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	
	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	
	return nil
}

// LoadConfigFromEnv loads configuration from environment variables
func LoadConfigFromEnv() *FuseConfig {
	config := DefaultFuseConfig()
	
	// Cache settings
	if val := os.Getenv("NOISEFS_CACHE_DIR_MAX_SIZE"); val != "" {
		if size, err := strconv.Atoi(val); err == nil && size > 0 {
			config.Cache.DirectoryMaxSize = size
		}
	}
	if val := os.Getenv("NOISEFS_CACHE_DIR_TTL"); val != "" {
		if ttl, err := time.ParseDuration(val); err == nil && ttl > 0 {
			config.Cache.DirectoryTTL = ttl
		}
	}
	if val := os.Getenv("NOISEFS_CACHE_ENABLE_METRICS"); val != "" {
		config.Cache.DirectoryEnableMetrics = val == "true" || val == "1"
	}
	
	// Security settings
	if val := os.Getenv("NOISEFS_ENABLE_ENCRYPTION"); val != "" {
		config.Security.EnableEncryption = val == "true" || val == "1"
	}
	if val := os.Getenv("NOISEFS_SECURE_MEMORY"); val != "" {
		config.Security.SecureMemoryLocking = val == "true" || val == "1"
	}
	if val := os.Getenv("NOISEFS_SECURE_DELETION"); val != "" {
		config.Security.SecureDeletion = val == "true" || val == "1"
	}
	
	// Performance settings
	if val := os.Getenv("NOISEFS_MAX_CONCURRENT_OPS"); val != "" {
		if ops, err := strconv.Atoi(val); err == nil && ops > 0 {
			config.Performance.MaxConcurrentOperations = ops
		}
	}
	if val := os.Getenv("NOISEFS_READ_BUFFER_SIZE"); val != "" {
		if size, err := strconv.Atoi(val); err == nil && size > 0 {
			config.Performance.ReadBufferSize = size
		}
	}
	if val := os.Getenv("NOISEFS_WRITE_BUFFER_SIZE"); val != "" {
		if size, err := strconv.Atoi(val); err == nil && size > 0 {
			config.Performance.WriteBufferSize = size
		}
	}
	
	// Mount settings
	if val := os.Getenv("NOISEFS_ALLOW_OTHER"); val != "" {
		config.Mount.AllowOther = val == "true" || val == "1"
	}
	if val := os.Getenv("NOISEFS_DEBUG"); val != "" {
		config.Mount.Debug = val == "true" || val == "1"
	}
	if val := os.Getenv("NOISEFS_READ_ONLY"); val != "" {
		config.Mount.ReadOnly = val == "true" || val == "1"
	}
	if val := os.Getenv("NOISEFS_VOLUME_NAME"); val != "" {
		config.Mount.DefaultVolumeName = val
	}
	
	return config
}

// GetDefaultConfigPath returns the default configuration file path
func GetDefaultConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	
	configDir := filepath.Join(homeDir, ".noisefs")
	return filepath.Join(configDir, "config.json"), nil
}

// LoadConfig loads configuration from file if it exists, otherwise returns default config
func LoadConfig() (*FuseConfig, error) {
	// Try to load from environment first
	if configPath := os.Getenv("NOISEFS_CONFIG_PATH"); configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			return LoadConfigFromFile(configPath)
		}
	}
	
	// Try default config path
	defaultPath, err := GetDefaultConfigPath()
	if err != nil {
		return nil, err
	}
	
	if _, err := os.Stat(defaultPath); err == nil {
		return LoadConfigFromFile(defaultPath)
	}
	
	// Return default config with environment overrides
	config := LoadConfigFromEnv()
	if err := ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid default configuration: %w", err)
	}
	
	return config, nil
}