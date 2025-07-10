package blocks

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestNewBlock(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "valid block",
			data:    []byte("hello world"),
			wantErr: false,
		},
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "nil data",
			data:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block, err := NewBlock(tt.data)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewBlock() error = nil, wantErr %v", tt.wantErr)
				}
				return
			}
			
			if err != nil {
				t.Errorf("NewBlock() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if block == nil {
				t.Error("NewBlock() returned nil block")
				return
			}
			
			if !bytes.Equal(block.Data, tt.data) {
				t.Errorf("NewBlock() data = %v, want %v", block.Data, tt.data)
			}
			
			// Verify the ID is correct SHA-256 hash
			expectedHash := sha256.Sum256(tt.data)
			expectedID := hex.EncodeToString(expectedHash[:])
			if block.ID != expectedID {
				t.Errorf("NewBlock() ID = %v, want %v", block.ID, expectedID)
			}
		})
	}
}

func TestNewRandomBlock(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantErr bool
	}{
		{
			name:    "valid size",
			size:    1024,
			wantErr: false,
		},
		{
			name:    "zero size",
			size:    0,
			wantErr: true,
		},
		{
			name:    "negative size",
			size:    -1,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block, err := NewRandomBlock(tt.size)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewRandomBlock() error = nil, wantErr %v", tt.wantErr)
				}
				return
			}
			
			if err != nil {
				t.Errorf("NewRandomBlock() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if block == nil {
				t.Error("NewRandomBlock() returned nil block")
				return
			}
			
			if len(block.Data) != tt.size {
				t.Errorf("NewRandomBlock() size = %v, want %v", len(block.Data), tt.size)
			}
			
			// Verify the data appears random (not all zeros)
			allZeros := true
			for _, b := range block.Data {
				if b != 0 {
					allZeros = false
					break
				}
			}
			if allZeros && tt.size > 0 {
				t.Error("NewRandomBlock() generated all zeros, likely not random")
			}
		})
	}
}

func TestBlockXOR(t *testing.T) {
	tests := []struct {
		name    string
		data1   []byte
		data2   []byte
		want    []byte
		wantErr bool
	}{
		{
			name:    "same size blocks",
			data1:   []byte{0x01, 0x02, 0x03},
			data2:   []byte{0x04, 0x05, 0x06},
			want:    []byte{0x05, 0x07, 0x05}, // XOR result
			wantErr: false,
		},
		{
			name:    "different size blocks",
			data1:   []byte{0x01, 0x02},
			data2:   []byte{0x04, 0x05, 0x06},
			wantErr: true,
		},
		{
			name:    "XOR with self gives zeros",
			data1:   []byte{0x01, 0x02, 0x03},
			data2:   []byte{0x01, 0x02, 0x03},
			want:    []byte{0x00, 0x00, 0x00},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block1, err := NewBlock(tt.data1)
			if err != nil {
				t.Fatalf("Failed to create block1: %v", err)
			}
			
			block2, err := NewBlock(tt.data2)
			if err != nil {
				t.Fatalf("Failed to create block2: %v", err)
			}
			
			result, err := block1.XOR(block2)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("Block.XOR() error = nil, wantErr %v", tt.wantErr)
				}
				return
			}
			
			if err != nil {
				t.Errorf("Block.XOR() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if !bytes.Equal(result.Data, tt.want) {
				t.Errorf("Block.XOR() = %v, want %v", result.Data, tt.want)
			}
		})
	}
}

func TestBlockVerifyIntegrity(t *testing.T) {
	data := []byte("test data")
	
	// Create a block normally
	block, err := NewBlock(data)
	if err != nil {
		t.Fatalf("Failed to create block: %v", err)
	}
	
	// Should verify correctly
	if !block.VerifyIntegrity() {
		t.Error("Block.VerifyIntegrity() = false, want true for valid block")
	}
	
	// Corrupt the ID
	block.ID = "invalid_id"
	if block.VerifyIntegrity() {
		t.Error("Block.VerifyIntegrity() = true, want false for corrupted ID")
	}
	
	// Corrupt the data but fix the ID
	block.Data = []byte("corrupted data")
	if block.VerifyIntegrity() {
		t.Error("Block.VerifyIntegrity() = true, want false for corrupted data")
	}
}

func TestBlockSize(t *testing.T) {
	data := []byte("hello world")
	block, err := NewBlock(data)
	if err != nil {
		t.Fatalf("Failed to create block: %v", err)
	}
	
	if block.Size() != len(data) {
		t.Errorf("Block.Size() = %v, want %v", block.Size(), len(data))
	}
}

func TestBlockXOR3(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		rand1   []byte
		rand2   []byte
		want    []byte
		wantErr bool
	}{
		{
			name:    "same size blocks",
			data:    []byte{0x01, 0x02, 0x03},
			rand1:   []byte{0x04, 0x05, 0x06},
			rand2:   []byte{0x07, 0x08, 0x09},
			want:    []byte{0x02, 0x0F, 0x0C}, // 0x01^0x04^0x07, 0x02^0x05^0x08, 0x03^0x06^0x09
			wantErr: false,
		},
		{
			name:    "different size data and rand1",
			data:    []byte{0x01, 0x02},
			rand1:   []byte{0x04, 0x05, 0x06},
			rand2:   []byte{0x07, 0x08},
			wantErr: true,
		},
		{
			name:    "different size data and rand2",
			data:    []byte{0x01, 0x02, 0x03},
			rand1:   []byte{0x04, 0x05, 0x06},
			rand2:   []byte{0x07, 0x08},
			wantErr: true,
		},
		{
			name:    "XOR3 with zeros gives original",
			data:    []byte{0x01, 0x02, 0x03},
			rand1:   []byte{0x00, 0x00, 0x00},
			rand2:   []byte{0x00, 0x00, 0x00},
			want:    []byte{0x01, 0x02, 0x03},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataBlock, err := NewBlock(tt.data)
			if err != nil {
				t.Fatalf("Failed to create data block: %v", err)
			}
			
			rand1Block, err := NewBlock(tt.rand1)
			if err != nil {
				t.Fatalf("Failed to create randomizer1 block: %v", err)
			}
			
			rand2Block, err := NewBlock(tt.rand2)
			if err != nil {
				t.Fatalf("Failed to create randomizer2 block: %v", err)
			}
			
			result, err := dataBlock.XOR3(rand1Block, rand2Block)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("Block.XOR3() error = nil, wantErr %v", tt.wantErr)
				}
				return
			}
			
			if err != nil {
				t.Errorf("Block.XOR3() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if !bytes.Equal(result.Data, tt.want) {
				t.Errorf("Block.XOR3() = %v, want %v", result.Data, tt.want)
			}
		})
	}
}

func TestXOR3Reversibility(t *testing.T) {
	// Test that XOR3 operations are reversible (A XOR B XOR C XOR B XOR C = A)
	original := []byte("original data123")
	randomizer1 := []byte("random key1!!!!!")
	randomizer2 := []byte("random key2!!!!!")
	
	if len(original) != len(randomizer1) || len(original) != len(randomizer2) {
		t.Fatal("Test data must be same length")
	}
	
	origBlock, err := NewBlock(original)
	if err != nil {
		t.Fatalf("Failed to create original block: %v", err)
	}
	
	rand1Block, err := NewBlock(randomizer1)
	if err != nil {
		t.Fatalf("Failed to create randomizer1 block: %v", err)
	}
	
	rand2Block, err := NewBlock(randomizer2)
	if err != nil {
		t.Fatalf("Failed to create randomizer2 block: %v", err)
	}
	
	// Encrypt: original XOR randomizer1 XOR randomizer2
	encrypted, err := origBlock.XOR3(rand1Block, rand2Block)
	if err != nil {
		t.Fatalf("Failed to XOR3 encrypt: %v", err)
	}
	
	// Decrypt: encrypted XOR randomizer1 XOR randomizer2
	decrypted, err := encrypted.XOR3(rand1Block, rand2Block)
	if err != nil {
		t.Fatalf("Failed to XOR3 decrypt: %v", err)
	}
	
	// Should get back original data
	if !bytes.Equal(decrypted.Data, original) {
		t.Errorf("XOR3 is not reversible: got %v, want %v", decrypted.Data, original)
	}
}

func TestXORReversibility(t *testing.T) {
	// Test that XOR operations are reversible (A XOR B XOR B = A)
	original := []byte("original data")
	randomizer := []byte("random key!!!")
	
	if len(original) != len(randomizer) {
		t.Fatal("Test data must be same length")
	}
	
	origBlock, err := NewBlock(original)
	if err != nil {
		t.Fatalf("Failed to create original block: %v", err)
	}
	
	randBlock, err := NewBlock(randomizer)
	if err != nil {
		t.Fatalf("Failed to create randomizer block: %v", err)
	}
	
	// Encrypt: original XOR randomizer
	encrypted, err := origBlock.XOR(randBlock)
	if err != nil {
		t.Fatalf("Failed to XOR encrypt: %v", err)
	}
	
	// Decrypt: encrypted XOR randomizer
	decrypted, err := encrypted.XOR(randBlock)
	if err != nil {
		t.Fatalf("Failed to XOR decrypt: %v", err)
	}
	
	// Should get back original data
	if !bytes.Equal(decrypted.Data, original) {
		t.Errorf("XOR is not reversible: got %v, want %v", decrypted.Data, original)
	}
}