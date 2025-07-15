package util

import (
	"encoding/json"
	"os"
)

// JSONOutput provides structured output for CLI operations
type JSONOutput struct {
	Success bool                   `json:"success"`
	Error   string                 `json:"error,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// UploadResult represents the result of an upload operation
type UploadResult struct {
	DescriptorCID string `json:"descriptor_cid"`
	Filename      string `json:"filename"`
	FileSize      int64  `json:"file_size"`
	BlockCount    int    `json:"block_count"`
	BlockSize     int    `json:"block_size"`
}

// DownloadResult represents the result of a download operation
type DownloadResult struct {
	OutputPath    string `json:"output_path"`
	Filename      string `json:"filename"`
	FileSize      int64  `json:"file_size"`
	BlockCount    int    `json:"block_count"`
}

// DirectoryUploadResult represents the result of a directory upload operation
type DirectoryUploadResult struct {
	DirectoryCID  string `json:"directory_cid"`
	DirectoryPath string `json:"directory_path"`
	TotalFiles    int    `json:"total_files"`
	TotalSize     int64  `json:"total_size"`
	BlockSize     int    `json:"block_size"`
}

// StatsResult represents system statistics
type StatsResult struct {
	IPFS       IPFSStats          `json:"ipfs"`
	Cache      CacheStats         `json:"cache"`
	Blocks     BlockStats         `json:"blocks"`
	Storage    StorageStats       `json:"storage"`
	Activity   ActivityStats      `json:"activity"`
	Altruistic *AltruisticStats   `json:"altruistic,omitempty"`
}

// IPFSStats represents IPFS connection information
type IPFSStats struct {
	Connected bool `json:"connected"`
	Peers     int  `json:"peer_count"`
}

// CacheStats represents cache performance metrics
type CacheStats struct {
	Size      int     `json:"size"`
	Hits      int64   `json:"hits"`
	Misses    int64   `json:"misses"`
	Evictions int64   `json:"evictions"`
	HitRate   float64 `json:"hit_rate"`
}

// BlockStats represents block management metrics
type BlockStats struct {
	Reused   int64   `json:"reused"`
	Generated int64   `json:"generated"`
	ReuseRate float64 `json:"reuse_rate"`
}

// StorageStats represents storage efficiency metrics
type StorageStats struct {
	OriginalBytes int64   `json:"original_bytes"`
	StoredBytes   int64   `json:"stored_bytes"`
	Overhead      float64 `json:"overhead_percent"`
}

// ActivityStats represents operation counts
type ActivityStats struct {
	Uploads   int64 `json:"uploads"`
	Downloads int64 `json:"downloads"`
}

// AltruisticStats represents altruistic cache statistics
type AltruisticStats struct {
	Enabled              bool    `json:"enabled"`
	PersonalBlocks       int     `json:"personal_blocks"`
	AltruisticBlocks     int     `json:"altruistic_blocks"`
	PersonalSize         int64   `json:"personal_size"`
	AltruisticSize       int64   `json:"altruistic_size"`
	TotalCapacity        int64   `json:"total_capacity"`
	PersonalPercent      float64 `json:"personal_percent"`
	AltruisticPercent    float64 `json:"altruistic_percent"`
	UsedPercent          float64 `json:"used_percent"`
	PersonalHitRate      float64 `json:"personal_hit_rate"`
	AltruisticHitRate    float64 `json:"altruistic_hit_rate"`
	FlexPoolUsage        float64 `json:"flex_pool_usage"`
	MinPersonalCacheMB   int     `json:"min_personal_cache_mb"`
}

// PrintJSON outputs data as formatted JSON
func PrintJSON(data interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// PrintJSONError outputs an error in JSON format
func PrintJSONError(err error) {
	output := JSONOutput{
		Success: false,
		Error:   err.Error(),
	}
	json.NewEncoder(os.Stdout).Encode(output)
}

// PrintJSONSuccess outputs success data in JSON format
func PrintJSONSuccess(data interface{}) {
	output := JSONOutput{
		Success: true,
		Data:    map[string]interface{}{"result": data},
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(output)
}