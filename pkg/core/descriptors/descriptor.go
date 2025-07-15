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
		Version:   "3.0",
		Filename:  filename,
		FileSize:  fileSize,
		BlockSize: blockSize,
		Blocks:    make([]BlockPair, 0),
		CreatedAt: time.Now(),
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