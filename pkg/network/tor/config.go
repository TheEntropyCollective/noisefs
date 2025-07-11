package tor

import (
	"time"
)

// Config holds Tor client configuration
type Config struct {
	// Basic Tor settings
	Enabled      bool   `json:"enabled" yaml:"enabled"`
	SOCKSProxy   string `json:"socks_proxy" yaml:"socks_proxy"`     // Default: 127.0.0.1:9050
	ControlPort  string `json:"control_port" yaml:"control_port"`   // Default: 127.0.0.1:9051
	ControlPass  string `json:"control_pass" yaml:"control_pass"`   // Optional auth
	
	// Upload settings (default: enabled for privacy)
	Upload UploadConfig `json:"upload" yaml:"upload"`
	
	// Download settings (default: disabled for performance)
	Download DownloadConfig `json:"download" yaml:"download"`
	
	// Announcement settings
	Announce AnnounceConfig `json:"announce" yaml:"announce"`
	
	// Circuit management
	CircuitPool CircuitPoolConfig `json:"circuit_pool" yaml:"circuit_pool"`
	
	// Performance settings
	Performance PerformanceConfig `json:"performance" yaml:"performance"`
}

// UploadConfig controls Tor usage for uploads
type UploadConfig struct {
	Enabled          bool          `json:"enabled" yaml:"enabled"`                       // Default: true
	CircuitRotation  time.Duration `json:"circuit_rotation" yaml:"circuit_rotation"`    // Default: 10m
	JitterMin        time.Duration `json:"jitter_min" yaml:"jitter_min"`               // Default: 1s
	JitterMax        time.Duration `json:"jitter_max" yaml:"jitter_max"`               // Default: 5s
	SplitCircuits    bool          `json:"split_circuits" yaml:"split_circuits"`       // Use different circuits for blocks
}

// DownloadConfig controls Tor usage for downloads
type DownloadConfig struct {
	Enabled       bool `json:"enabled" yaml:"enabled"`               // Default: false
	CircuitReuse  bool `json:"circuit_reuse" yaml:"circuit_reuse"`   // Reuse circuits for performance
	OnlyMetadata  bool `json:"only_metadata" yaml:"only_metadata"`   // Only use Tor for descriptors
}

// AnnounceConfig controls Tor usage for announcements
type AnnounceConfig struct {
	Enabled          bool `json:"enabled" yaml:"enabled"`                     // Default: true
	UseHiddenService bool `json:"use_hidden_service" yaml:"use_hidden_service"` // Publish via .onion
}

// CircuitPoolConfig manages circuit pool behavior
type CircuitPoolConfig struct {
	MinCircuits      int           `json:"min_circuits" yaml:"min_circuits"`           // Minimum pre-established
	MaxCircuits      int           `json:"max_circuits" yaml:"max_circuits"`           // Maximum concurrent
	CircuitLifetime  time.Duration `json:"circuit_lifetime" yaml:"circuit_lifetime"`   // Max circuit age
	BuildTimeout     time.Duration `json:"build_timeout" yaml:"build_timeout"`         // Circuit creation timeout
	HealthCheckInterval time.Duration `json:"health_check" yaml:"health_check"`        // Circuit health checks
}

// PerformanceConfig contains performance tuning settings
type PerformanceConfig struct {
	// PERFORMANCE IMPACT: These settings significantly affect speed
	ConcurrentUploads   int           `json:"concurrent_uploads" yaml:"concurrent_uploads"`     // Default: 3 (vs 10 without Tor)
	RequestTimeout      time.Duration `json:"request_timeout" yaml:"request_timeout"`         // Default: 60s (vs 30s)
	RetryAttempts       int           `json:"retry_attempts" yaml:"retry_attempts"`           // Default: 5 (vs 3)
	StreamBufferSize    int           `json:"stream_buffer_size" yaml:"stream_buffer_size"`   // Default: 32KB
	
	// Tor-specific optimizations
	UseCompression      bool          `json:"use_compression" yaml:"use_compression"`         // Compress before sending
	ParallelCircuits    int           `json:"parallel_circuits" yaml:"parallel_circuits"`     // For large uploads
}

// DefaultConfig returns production-ready Tor configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:     true,
		SOCKSProxy:  "127.0.0.1:9050",
		ControlPort: "127.0.0.1:9051",
		
		Upload: UploadConfig{
			Enabled:         true,  // ON by default for privacy
			CircuitRotation: 10 * time.Minute,
			JitterMin:       1 * time.Second,
			JitterMax:       5 * time.Second,
			SplitCircuits:   true,  // Different circuits for each block
		},
		
		Download: DownloadConfig{
			Enabled:      false, // OFF by default for performance
			CircuitReuse: true,  // If enabled, reuse circuits
			OnlyMetadata: false, // Full file download through Tor
		},
		
		Announce: AnnounceConfig{
			Enabled:          true,
			UseHiddenService: false, // Requires additional setup
		},
		
		CircuitPool: CircuitPoolConfig{
			MinCircuits:         3,
			MaxCircuits:         10,
			CircuitLifetime:     30 * time.Minute,
			BuildTimeout:        30 * time.Second,
			HealthCheckInterval: 2 * time.Minute,
		},
		
		Performance: PerformanceConfig{
			// PERFORMANCE IMPACT: Reduced concurrency for Tor
			ConcurrentUploads: 3,    // vs 10 without Tor (3.3x slower)
			RequestTimeout:    60 * time.Second, // vs 30s (2x slower timeout)
			RetryAttempts:     5,    // More retries for Tor reliability
			StreamBufferSize:  32 * 1024,
			UseCompression:    true, // Helps with Tor bandwidth
			ParallelCircuits:  3,    // For splitting large uploads
		},
	}
}

// PerformanceImpact returns estimated performance impact
func (c *Config) PerformanceImpact() map[string]float64 {
	impacts := make(map[string]float64)
	
	if c.Upload.Enabled {
		// PERFORMANCE IMPACT: Upload speeds
		baseImpact := 3.0 // 3x slower base
		if c.Upload.SplitCircuits {
			baseImpact *= 1.2 // 20% additional overhead
		}
		impacts["upload_speed"] = baseImpact
		
		// Jitter adds latency
		avgJitter := (c.Upload.JitterMin + c.Upload.JitterMax) / 2
		impacts["upload_latency"] = float64(avgJitter.Seconds())
	}
	
	if c.Download.Enabled {
		// PERFORMANCE IMPACT: Download speeds
		impacts["download_speed"] = 2.5 // 2.5x slower
		if !c.Download.CircuitReuse {
			impacts["download_speed"] = 4.0 // 4x slower without reuse
		}
	}
	
	// Circuit establishment overhead
	impacts["initial_connection"] = float64(c.CircuitPool.BuildTimeout.Seconds())
	
	return impacts
}

// Validate ensures configuration is valid
func (c *Config) Validate() error {
	// Validate basic settings
	if c.SOCKSProxy == "" {
		c.SOCKSProxy = "127.0.0.1:9050"
	}
	
	// Validate performance settings
	if c.Performance.ConcurrentUploads < 1 {
		c.Performance.ConcurrentUploads = 1
	}
	if c.Performance.ConcurrentUploads > 10 {
		// PERFORMANCE WARNING: Too many circuits can overload Tor
		c.Performance.ConcurrentUploads = 10
	}
	
	// Validate circuit pool
	if c.CircuitPool.MinCircuits < 1 {
		c.CircuitPool.MinCircuits = 1
	}
	if c.CircuitPool.MaxCircuits < c.CircuitPool.MinCircuits {
		c.CircuitPool.MaxCircuits = c.CircuitPool.MinCircuits * 2
	}
	
	return nil
}