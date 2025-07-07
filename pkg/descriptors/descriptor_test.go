package descriptors

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewDescriptor(t *testing.T) {
	filename := "test.txt"
	fileSize := int64(1024)
	blockSize := 128

	desc := NewDescriptor(filename, fileSize, blockSize)

	if desc == nil {
		t.Fatal("NewDescriptor() returned nil")
	}

	if desc.Version != "2.0" {
		t.Errorf("NewDescriptor() Version = %v, want 2.0", desc.Version)
	}

	if desc.Filename != filename {
		t.Errorf("NewDescriptor() Filename = %v, want %v", desc.Filename, filename)
	}

	if desc.FileSize != fileSize {
		t.Errorf("NewDescriptor() FileSize = %v, want %v", desc.FileSize, fileSize)
	}

	if desc.BlockSize != blockSize {
		t.Errorf("NewDescriptor() BlockSize = %v, want %v", desc.BlockSize, blockSize)
	}

	if len(desc.Blocks) != 0 {
		t.Errorf("NewDescriptor() Blocks length = %v, want 0", len(desc.Blocks))
	}

	// Check that CreatedAt is recent (within last minute)
	if time.Since(desc.CreatedAt) > time.Minute {
		t.Errorf("NewDescriptor() CreatedAt seems too old: %v", desc.CreatedAt)
	}
}

func TestDescriptorAddBlockPair(t *testing.T) {
	desc := NewDescriptor("test.txt", 1024, 128)

	// Test valid block pair
	err := desc.AddBlockPair("data_cid_1", "rand_cid_1")
	if err != nil {
		t.Errorf("AddBlockPair() error = %v, want nil", err)
	}

	if len(desc.Blocks) != 1 {
		t.Errorf("After AddBlockPair(), Blocks length = %v, want 1", len(desc.Blocks))
	}

	if desc.Blocks[0].DataCID != "data_cid_1" {
		t.Errorf("Block DataCID = %v, want data_cid_1", desc.Blocks[0].DataCID)
	}

	if desc.Blocks[0].RandomizerCID1 != "rand_cid_1" {
		t.Errorf("Block RandomizerCID1 = %v, want rand_cid_1", desc.Blocks[0].RandomizerCID1)
	}

	// Should be empty for legacy 2-tuple
	if desc.Blocks[0].RandomizerCID2 != "" {
		t.Errorf("Block RandomizerCID2 = %v, want empty string for legacy 2-tuple", desc.Blocks[0].RandomizerCID2)
	}

	// Test empty data CID
	err = desc.AddBlockPair("", "rand_cid_2")
	if err == nil {
		t.Error("AddBlockPair() with empty data CID should return error")
	}

	// Test empty randomizer CID
	err = desc.AddBlockPair("data_cid_2", "")
	if err == nil {
		t.Error("AddBlockPair() with empty randomizer CID should return error")
	}

	// Length should still be 1 (failed additions shouldn't be added)
	if len(desc.Blocks) != 1 {
		t.Errorf("After failed AddBlockPair(), Blocks length = %v, want 1", len(desc.Blocks))
	}

	// Test adding multiple valid pairs
	err = desc.AddBlockPair("data_cid_2", "rand_cid_2")
	if err != nil {
		t.Errorf("AddBlockPair() second pair error = %v, want nil", err)
	}

	if len(desc.Blocks) != 2 {
		t.Errorf("After second AddBlockPair(), Blocks length = %v, want 2", len(desc.Blocks))
	}
}

func TestDescriptorValidate(t *testing.T) {
	tests := []struct {
		name    string
		desc    *Descriptor
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid descriptor",
			desc: &Descriptor{
				Version:   "1.0",
				Filename:  "test.txt",
				FileSize:  1024,
				BlockSize: 128,
				Blocks: []BlockPair{
					{DataCID: "data1", RandomizerCID1: "rand1", RandomizerCID2: ""},
				},
				CreatedAt: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "empty version",
			desc: &Descriptor{
				Version:   "",
				Filename:  "test.txt",
				FileSize:  1024,
				BlockSize: 128,
				Blocks: []BlockPair{
					{DataCID: "data1", RandomizerCID1: "rand1", RandomizerCID2: ""},
				},
			},
			wantErr: true,
			errMsg:  "version is required",
		},
		{
			name: "empty filename",
			desc: &Descriptor{
				Version:   "1.0",
				Filename:  "",
				FileSize:  1024,
				BlockSize: 128,
				Blocks: []BlockPair{
					{DataCID: "data1", RandomizerCID1: "rand1", RandomizerCID2: ""},
				},
			},
			wantErr: true,
			errMsg:  "filename is required",
		},
		{
			name: "zero file size",
			desc: &Descriptor{
				Version:   "1.0",
				Filename:  "test.txt",
				FileSize:  0,
				BlockSize: 128,
				Blocks: []BlockPair{
					{DataCID: "data1", RandomizerCID1: "rand1", RandomizerCID2: ""},
				},
			},
			wantErr: true,
			errMsg:  "file size must be positive",
		},
		{
			name: "negative file size",
			desc: &Descriptor{
				Version:   "1.0",
				Filename:  "test.txt",
				FileSize:  -1,
				BlockSize: 128,
				Blocks: []BlockPair{
					{DataCID: "data1", RandomizerCID1: "rand1", RandomizerCID2: ""},
				},
			},
			wantErr: true,
			errMsg:  "file size must be positive",
		},
		{
			name: "zero block size",
			desc: &Descriptor{
				Version:   "1.0",
				Filename:  "test.txt",
				FileSize:  1024,
				BlockSize: 0,
				Blocks: []BlockPair{
					{DataCID: "data1", RandomizerCID1: "rand1", RandomizerCID2: ""},
				},
			},
			wantErr: true,
			errMsg:  "block size must be positive",
		},
		{
			name: "no blocks",
			desc: &Descriptor{
				Version:   "1.0",
				Filename:  "test.txt",
				FileSize:  1024,
				BlockSize: 128,
				Blocks:    []BlockPair{},
			},
			wantErr: true,
			errMsg:  "must contain at least one block",
		},
		{
			name: "empty data CID in block",
			desc: &Descriptor{
				Version:   "1.0",
				Filename:  "test.txt",
				FileSize:  1024,
				BlockSize: 128,
				Blocks: []BlockPair{
					{DataCID: "", RandomizerCID1: "rand1", RandomizerCID2: ""},
				},
			},
			wantErr: true,
			errMsg:  "block CIDs cannot be empty",
		},
		{
			name: "empty randomizer CID in block",
			desc: &Descriptor{
				Version:   "1.0",
				Filename:  "test.txt",
				FileSize:  1024,
				BlockSize: 128,
				Blocks: []BlockPair{
					{DataCID: "data1", RandomizerCID1: "", RandomizerCID2: ""},
				},
			},
			wantErr: true,
			errMsg:  "block CIDs cannot be empty",
		},
		{
			name: "same data and randomizer CID",
			desc: &Descriptor{
				Version:   "1.0",
				Filename:  "test.txt",
				FileSize:  1024,
				BlockSize: 128,
				Blocks: []BlockPair{
					{DataCID: "same_cid", RandomizerCID1: "same_cid", RandomizerCID2: ""},
				},
			},
			wantErr: true,
			errMsg:  "data and randomizer CIDs must be different",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.desc.Validate()

			if tt.wantErr && err == nil {
				t.Errorf("Validate() error = nil, wantErr %v", tt.wantErr)
				return
			}

			if !tt.wantErr && err != nil {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, want error containing %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestDescriptorToJSON(t *testing.T) {
	desc := NewDescriptor("test.txt", 1024, 128)
	err := desc.AddBlockTriple("data1", "rand1", "rand2")
	if err != nil {
		t.Fatalf("Failed to add block triple: %v", err)
	}

	jsonData, err := desc.ToJSON()
	if err != nil {
		t.Errorf("ToJSON() error = %v, want nil", err)
	}

	if len(jsonData) == 0 {
		t.Error("ToJSON() returned empty data")
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	err = json.Unmarshal(jsonData, &parsed)
	if err != nil {
		t.Errorf("ToJSON() produced invalid JSON: %v", err)
	}

	// Check some expected fields
	if parsed["filename"] != "test.txt" {
		t.Errorf("JSON filename = %v, want test.txt", parsed["filename"])
	}

	if parsed["file_size"] != float64(1024) {
		t.Errorf("JSON file_size = %v, want 1024", parsed["file_size"])
	}

	// Test invalid descriptor
	invalidDesc := &Descriptor{} // Missing required fields
	_, err = invalidDesc.ToJSON()
	if err == nil {
		t.Error("ToJSON() with invalid descriptor should return error")
	}
}

func TestDescriptorFromJSON(t *testing.T) {
	// Create valid JSON for 3-tuple format
	validJSON := `{
		"version": "2.0",
		"filename": "test.txt",
		"file_size": 1024,
		"block_size": 128,
		"blocks": [
			{
				"data_cid": "data1",
				"randomizer_cid1": "rand1",
				"randomizer_cid2": "rand2"
			}
		],
		"created_at": "2023-01-01T00:00:00Z"
	}`

	desc, err := FromJSON([]byte(validJSON))
	if err != nil {
		t.Errorf("FromJSON() error = %v, want nil", err)
	}

	if desc == nil {
		t.Fatal("FromJSON() returned nil")
	}

	if desc.Filename != "test.txt" {
		t.Errorf("FromJSON() Filename = %v, want test.txt", desc.Filename)
	}

	if desc.FileSize != 1024 {
		t.Errorf("FromJSON() FileSize = %v, want 1024", desc.FileSize)
	}

	if len(desc.Blocks) != 1 {
		t.Errorf("FromJSON() Blocks length = %v, want 1", len(desc.Blocks))
	}

	// Test empty JSON
	_, err = FromJSON([]byte{})
	if err == nil {
		t.Error("FromJSON() with empty data should return error")
	}

	// Test invalid JSON
	_, err = FromJSON([]byte("invalid json"))
	if err == nil {
		t.Error("FromJSON() with invalid JSON should return error")
	}

	// Test JSON with invalid descriptor data
	invalidJSON := `{
		"version": "",
		"filename": "test.txt",
		"file_size": 1024,
		"block_size": 128,
		"blocks": []
	}`

	_, err = FromJSON([]byte(invalidJSON))
	if err == nil {
		t.Error("FromJSON() with invalid descriptor should return error")
	}
}

func TestDescriptorRoundTrip(t *testing.T) {
	// Create a descriptor
	original := NewDescriptor("roundtrip.txt", 2048, 256)
	err := original.AddBlockTriple("data1", "rand1", "rand1b")
	if err != nil {
		t.Fatalf("Failed to add first block triple: %v", err)
	}
	err = original.AddBlockTriple("data2", "rand2", "rand2b")
	if err != nil {
		t.Fatalf("Failed to add second block triple: %v", err)
	}

	// Convert to JSON
	jsonData, err := original.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	// Convert back from JSON
	restored, err := FromJSON(jsonData)
	if err != nil {
		t.Fatalf("FromJSON() error = %v", err)
	}

	// Compare fields
	if restored.Version != original.Version {
		t.Errorf("Round-trip Version = %v, want %v", restored.Version, original.Version)
	}

	if restored.Filename != original.Filename {
		t.Errorf("Round-trip Filename = %v, want %v", restored.Filename, original.Filename)
	}

	if restored.FileSize != original.FileSize {
		t.Errorf("Round-trip FileSize = %v, want %v", restored.FileSize, original.FileSize)
	}

	if restored.BlockSize != original.BlockSize {
		t.Errorf("Round-trip BlockSize = %v, want %v", restored.BlockSize, original.BlockSize)
	}

	if len(restored.Blocks) != len(original.Blocks) {
		t.Errorf("Round-trip Blocks length = %v, want %v", len(restored.Blocks), len(original.Blocks))
	}

	for i, block := range restored.Blocks {
		if i >= len(original.Blocks) {
			break
		}
		origBlock := original.Blocks[i]
		if block.DataCID != origBlock.DataCID {
			t.Errorf("Round-trip Block[%d] DataCID = %v, want %v", i, block.DataCID, origBlock.DataCID)
		}
		if block.RandomizerCID1 != origBlock.RandomizerCID1 {
			t.Errorf("Round-trip Block[%d] RandomizerCID1 = %v, want %v", i, block.RandomizerCID1, origBlock.RandomizerCID1)
		}
	}
}

func TestBlockPairValidation(t *testing.T) {
	tests := []struct {
		name         string
		dataCID      string
		randomizerCID string
		wantErr      bool
	}{
		{
			name:         "valid pair",
			dataCID:      "data_cid",
			randomizerCID: "rand_cid",
			wantErr:      false,
		},
		{
			name:         "empty data CID",
			dataCID:      "",
			randomizerCID: "rand_cid",
			wantErr:      true,
		},
		{
			name:         "empty randomizer CID",
			dataCID:      "data_cid",
			randomizerCID: "",
			wantErr:      true,
		},
		{
			name:         "both empty",
			dataCID:      "",
			randomizerCID: "",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := NewDescriptor("test.txt", 1024, 128)
			err := desc.AddBlockPair(tt.dataCID, tt.randomizerCID)

			if tt.wantErr && err == nil {
				t.Errorf("AddBlockPair() error = nil, wantErr %v", tt.wantErr)
			}

			if !tt.wantErr && err != nil {
				t.Errorf("AddBlockPair() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDescriptorAddBlockTriple(t *testing.T) {
	desc := NewDescriptor("test.txt", 1024, 128)

	// Test valid block triple
	err := desc.AddBlockTriple("data_cid_1", "rand_cid_1", "rand_cid_2")
	if err != nil {
		t.Errorf("AddBlockTriple() error = %v, want nil", err)
	}

	if len(desc.Blocks) != 1 {
		t.Errorf("After AddBlockTriple(), Blocks length = %v, want 1", len(desc.Blocks))
	}

	block := desc.Blocks[0]
	if block.DataCID != "data_cid_1" {
		t.Errorf("Block DataCID = %v, want data_cid_1", block.DataCID)
	}

	if block.RandomizerCID1 != "rand_cid_1" {
		t.Errorf("Block RandomizerCID1 = %v, want rand_cid_1", block.RandomizerCID1)
	}

	if block.RandomizerCID2 != "rand_cid_2" {
		t.Errorf("Block RandomizerCID2 = %v, want rand_cid_2", block.RandomizerCID2)
	}

	// Test empty data CID
	err = desc.AddBlockTriple("", "rand_cid_3", "rand_cid_4")
	if err == nil {
		t.Error("AddBlockTriple() with empty data CID should return error")
	}

	// Test empty first randomizer CID
	err = desc.AddBlockTriple("data_cid_2", "", "rand_cid_4")
	if err == nil {
		t.Error("AddBlockTriple() with empty randomizer1 CID should return error")
	}

	// Test empty second randomizer CID
	err = desc.AddBlockTriple("data_cid_2", "rand_cid_3", "")
	if err == nil {
		t.Error("AddBlockTriple() with empty randomizer2 CID should return error")
	}

	// Test duplicate CIDs
	err = desc.AddBlockTriple("same_cid", "same_cid", "rand_cid_4")
	if err == nil {
		t.Error("AddBlockTriple() with duplicate data and randomizer1 CIDs should return error")
	}

	err = desc.AddBlockTriple("data_cid_2", "same_cid", "same_cid")
	if err == nil {
		t.Error("AddBlockTriple() with duplicate randomizer CIDs should return error")
	}

	// Length should still be 1 (failed additions shouldn't be added)
	if len(desc.Blocks) != 1 {
		t.Errorf("After failed AddBlockTriple(), Blocks length = %v, want 1", len(desc.Blocks))
	}
}

func TestDescriptorIsThreeTuple(t *testing.T) {
	desc := NewDescriptor("test.txt", 1024, 128)
	
	if !desc.IsThreeTuple() {
		t.Error("NewDescriptor() should create 3-tuple descriptor by default")
	}

	// Test legacy descriptor
	legacyDesc := &Descriptor{Version: "1.0"}
	if legacyDesc.IsThreeTuple() {
		t.Error("Legacy descriptor (v1.0) should not be 3-tuple")
	}
}

func TestDescriptorGetRandomizerCIDs(t *testing.T) {
	desc := NewDescriptor("test.txt", 1024, 128)
	
	// Add a 3-tuple block
	err := desc.AddBlockTriple("data1", "rand1", "rand2")
	if err != nil {
		t.Fatalf("Failed to add block triple: %v", err)
	}

	// Test valid index
	cid1, cid2, err := desc.GetRandomizerCIDs(0)
	if err != nil {
		t.Errorf("GetRandomizerCIDs(0) error = %v, want nil", err)
	}

	if cid1 != "rand1" {
		t.Errorf("GetRandomizerCIDs(0) cid1 = %v, want rand1", cid1)
	}

	if cid2 != "rand2" {
		t.Errorf("GetRandomizerCIDs(0) cid2 = %v, want rand2", cid2)
	}

	// Test invalid index
	_, _, err = desc.GetRandomizerCIDs(-1)
	if err == nil {
		t.Error("GetRandomizerCIDs(-1) should return error")
	}

	_, _, err = desc.GetRandomizerCIDs(1)
	if err == nil {
		t.Error("GetRandomizerCIDs(1) should return error for out of range")
	}

	// Test legacy 2-tuple descriptor
	legacyDesc := &Descriptor{
		Version: "1.0",
		Blocks: []BlockPair{
			{DataCID: "data1", RandomizerCID1: "rand1", RandomizerCID2: ""},
		},
	}

	cid1, cid2, err = legacyDesc.GetRandomizerCIDs(0)
	if err != nil {
		t.Errorf("GetRandomizerCIDs(0) on legacy descriptor error = %v, want nil", err)
	}

	if cid1 != "rand1" {
		t.Errorf("Legacy GetRandomizerCIDs(0) cid1 = %v, want rand1", cid1)
	}

	if cid2 != "" {
		t.Errorf("Legacy GetRandomizerCIDs(0) cid2 = %v, want empty string", cid2)
	}
}