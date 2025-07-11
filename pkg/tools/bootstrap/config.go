package bootstrap

import (
	"time"
)

// Profile represents a seeding profile
type Profile string

const (
	// ProfileMinimal provides basic seeding with ~500MB of data
	ProfileMinimal Profile = "minimal"
	// ProfileStandard provides standard seeding with ~2GB of data
	ProfileStandard Profile = "standard"
	// ProfileMaximum provides maximum seeding with ~10GB of data
	ProfileMaximum Profile = "maximum"
)

// SeedConfig holds configuration for the seeding process
type SeedConfig struct {
	Profile      Profile
	OutputDir    string
	IPFSEndpoint string
	MaxSize      int64
	Parallel     int
	
	// Content options
	IncludeVideo bool
	VideoQuality string
	BlocksPerSize int
	
	// Advanced options
	MinPublicDomainRatio float64
	GenesisBlockCount    int
	PopularityThreshold  float64
}

// ContentSource represents a source of public domain content
type ContentSource struct {
	Name        string            `json:"name"`
	URL         string            `json:"url"`
	Type        string            `json:"type"`
	Size        int64             `json:"size"`
	License     string            `json:"license"`
	Metadata    map[string]string `json:"metadata"`
	LastUpdated time.Time         `json:"last_updated"`
}

// BlockStats holds statistics about generated blocks
type BlockStats struct {
	TotalBlocks           int
	PublicDomainBlocks    int
	BlocksBySize          map[int]int
	AverageReusePotential float64
	ContentTypes          map[string]int
}

// PoolValidation holds pool validation results
type PoolValidation struct {
	Valid                bool
	Issues               []string
	TotalBlocks          int
	PublicDomainRatio    float64
	BlockSizeCoverage    map[int]bool
	DiversityScore       float64
	MinimumRequirementsMet bool
}

// DownloadProgress tracks download progress
type DownloadProgress struct {
	TotalFiles      int
	CompletedFiles  int
	TotalBytes      int64
	DownloadedBytes int64
	CurrentFile     string
	StartTime       time.Time
	Errors          []error
}

// ContentManifest describes available content for a type
type ContentManifest struct {
	Type         string          `json:"type"`
	Description  string          `json:"description"`
	TotalSize    int64           `json:"total_size"`
	FileCount    int             `json:"file_count"`
	Sources      []ContentSource `json:"sources"`
	LastUpdated  time.Time       `json:"last_updated"`
}

// VideoSource represents a video content source
type VideoSource struct {
	ContentSource
	Duration    string   `json:"duration"`
	Resolution  string   `json:"resolution"`
	Format      string   `json:"format"`
	Subtitles   []string `json:"subtitles,omitempty"`
	Thumbnail   string   `json:"thumbnail,omitempty"`
}

// GetDefaultConfig returns a default seed configuration
func GetDefaultConfig() *SeedConfig {
	return &SeedConfig{
		Profile:              ProfileStandard,
		OutputDir:            "./seed-data",
		IPFSEndpoint:         "http://localhost:5001",
		MaxSize:              2 * 1024 * 1024 * 1024, // 2GB
		Parallel:             4,
		IncludeVideo:         true,
		VideoQuality:         "720p",
		BlocksPerSize:        500,
		MinPublicDomainRatio: 0.6,
		GenesisBlockCount:    50,
		PopularityThreshold:  0.1,
	}
}