package descriptors

import (
	"encoding/json"
	"errors"
	"time"
)

// BlockPair represents a data block and its corresponding randomizer
type BlockPair struct {
	DataCID       string `json:"data_cid"`
	RandomizerCID string `json:"randomizer_cid"`
}

// Descriptor contains metadata needed to reconstruct a file
type Descriptor struct {
	Version   string      `json:"version"`
	Filename  string      `json:"filename"`
	FileSize  int64       `json:"file_size"`
	BlockSize int         `json:"block_size"`
	Blocks    []BlockPair `json:"blocks"`
	CreatedAt time.Time   `json:"created_at"`
}

// NewDescriptor creates a new file descriptor
func NewDescriptor(filename string, fileSize int64, blockSize int) *Descriptor {
	return &Descriptor{
		Version:   "1.0",
		Filename:  filename,
		FileSize:  fileSize,
		BlockSize: blockSize,
		Blocks:    make([]BlockPair, 0),
		CreatedAt: time.Now(),
	}
}

// AddBlockPair adds a data/randomizer block pair to the descriptor
func (d *Descriptor) AddBlockPair(dataCID, randomizerCID string) error {
	if dataCID == "" || randomizerCID == "" {
		return errors.New("CIDs cannot be empty")
	}
	
	d.Blocks = append(d.Blocks, BlockPair{
		DataCID:       dataCID,
		RandomizerCID: randomizerCID,
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
		if block.DataCID == "" || block.RandomizerCID == "" {
			return errors.New("block CIDs cannot be empty")
		}
		if block.DataCID == block.RandomizerCID {
			return errors.New("data and randomizer CIDs must be different")
		}
		_ = i
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