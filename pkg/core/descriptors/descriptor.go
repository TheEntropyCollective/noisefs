package descriptors

import (
	"encoding/json"
	"errors"
	"time"
)

// BlockPair represents a data block and its corresponding randomizers (3-tuple)
type BlockPair struct {
	DataCID        string `json:"data_cid"`
	RandomizerCID1 string `json:"randomizer_cid1"`
	RandomizerCID2 string `json:"randomizer_cid2"`
}

// DescriptorType represents the type of descriptor
type DescriptorType string

const (
	// FileType represents a regular file descriptor
	FileType DescriptorType = "file"
	// DirectoryType represents a directory descriptor
	DirectoryType DescriptorType = "directory"
)

// Descriptor contains metadata needed to reconstruct a file or directory
type Descriptor struct {
	Version      string         `json:"version"`
	Type         DescriptorType `json:"type,omitempty"`         // Optional for backward compatibility
	Filename     string         `json:"filename"`
	FileSize     int64          `json:"file_size"`
	BlockSize    int            `json:"block_size"`
	Blocks       []BlockPair    `json:"blocks,omitempty"`        // Empty for directories
	ManifestCID  string         `json:"manifest_cid,omitempty"`  // Only for directories
	CreatedAt    time.Time      `json:"created_at"`
}

// NewDescriptor creates a new file descriptor
func NewDescriptor(filename string, fileSize int64, blockSize int) *Descriptor {
	return &Descriptor{
		Version:   "3.0",
		Type:      FileType,
		Filename:  filename,
		FileSize:  fileSize,
		BlockSize: blockSize,
		Blocks:    make([]BlockPair, 0),
		CreatedAt: time.Now(),
	}
}

// NewDirectoryDescriptor creates a new directory descriptor
func NewDirectoryDescriptor(dirname string, manifestCID string) *Descriptor {
	return &Descriptor{
		Version:     "4.0",
		Type:        DirectoryType,
		Filename:    dirname,
		FileSize:    0,              // Directories don't have a fixed size
		BlockSize:   0,              // Not applicable for directories
		ManifestCID: manifestCID,
		CreatedAt:   time.Now(),
	}
}


// AddBlockTriple adds a data block with two randomizers (3-tuple)
func (d *Descriptor) AddBlockTriple(dataCID, randomizerCID1, randomizerCID2 string) error {
	if dataCID == "" || randomizerCID1 == "" || randomizerCID2 == "" {
		return errors.New("all CIDs cannot be empty")
	}
	
	if dataCID == randomizerCID1 || dataCID == randomizerCID2 || randomizerCID1 == randomizerCID2 {
		return errors.New("all CIDs must be different")
	}
	
	d.Blocks = append(d.Blocks, BlockPair{
		DataCID:        dataCID,
		RandomizerCID1: randomizerCID1,
		RandomizerCID2: randomizerCID2,
	})
	
	return nil
}

// Validate checks if the descriptor is valid
func (d *Descriptor) Validate() error {
	if d.Version == "" {
		return errors.New("descriptor version is required")
	}
	
	if d.Filename == "" {
		return errors.New("filename is required")
	}
	
	// Handle backward compatibility - if Type is not set, assume it's a file
	if d.Type == "" {
		d.Type = FileType
	}
	
	// Validate based on type
	switch d.Type {
	case FileType:
		return d.validateFile()
	case DirectoryType:
		return d.validateDirectory()
	default:
		// For backward compatibility, treat unknown types as files for older versions
		d.Type = FileType
		return d.validateFile()
	}
}

// validateFile validates file-specific fields
func (d *Descriptor) validateFile() error {
	if d.FileSize <= 0 {
		return errors.New("file size must be positive")
	}
	
	if d.BlockSize <= 0 {
		return errors.New("block size must be positive")
	}
	
	if len(d.Blocks) == 0 {
		return errors.New("descriptor must contain at least one block")
	}
	
	for i, block := range d.Blocks {
		if block.DataCID == "" || block.RandomizerCID1 == "" || block.RandomizerCID2 == "" {
			return errors.New("all CIDs must be present")
		}
		
		if block.DataCID == block.RandomizerCID1 || block.DataCID == block.RandomizerCID2 || block.RandomizerCID1 == block.RandomizerCID2 {
			return errors.New("all CIDs must be different")
		}
		_ = i
	}
	
	return nil
}

// validateDirectory validates directory-specific fields
func (d *Descriptor) validateDirectory() error {
	if d.Version != "4.0" {
		return errors.New("directory descriptors require version 4.0")
	}
	
	if d.ManifestCID == "" {
		return errors.New("directory descriptor must have a manifest CID")
	}
	
	if len(d.Blocks) > 0 {
		return errors.New("directory descriptors should not contain blocks")
	}
	
	return nil
}

// ToJSON serializes the descriptor to JSON
func (d *Descriptor) ToJSON() ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	
	return json.MarshalIndent(d, "", "  ")
}

// Marshal serializes the descriptor to JSON (alias for ToJSON)
func (d *Descriptor) Marshal() ([]byte, error) {
	return d.ToJSON()
}

// FromJSON deserializes a descriptor from JSON
func FromJSON(data []byte) (*Descriptor, error) {
	if len(data) == 0 {
		return nil, errors.New("empty JSON data")
	}
	
	var desc Descriptor
	if err := json.Unmarshal(data, &desc); err != nil {
		return nil, err
	}
	
	if err := desc.Validate(); err != nil {
		return nil, err
	}
	
	return &desc, nil
}


// GetRandomizerCIDs returns the randomizer CIDs for a block at the given index
func (d *Descriptor) GetRandomizerCIDs(blockIndex int) (string, string, error) {
	if blockIndex < 0 || blockIndex >= len(d.Blocks) {
		return "", "", errors.New("block index out of range")
	}
	
	block := d.Blocks[blockIndex]
	return block.RandomizerCID1, block.RandomizerCID2, nil
}

// IsFile returns true if this is a file descriptor
func (d *Descriptor) IsFile() bool {
	// Handle backward compatibility - if Type is not set, assume it's a file
	if d.Type == "" {
		return true
	}
	return d.Type == FileType
}

// IsDirectory returns true if this is a directory descriptor
func (d *Descriptor) IsDirectory() bool {
	return d.Type == DirectoryType
}