package blocks

import (
	"bytes"
	"strings"
	"testing"
)

func TestSplitAlwaysPadded(t *testing.T) {
	tests := []struct {
		name      string
		blockSize int
		input     string
		expected  []int // Expected block sizes
	}{
		{
			name:      "exact block size",
			blockSize: 10,
			input:     "1234567890",
			expected:  []int{10}, // No padding needed
		},
		{
			name:      "needs padding",
			blockSize: 10,
			input:     "12345",
			expected:  []int{10}, // Padded to full size
		},
		{
			name:      "multiple blocks with padding",
			blockSize: 10,
			input:     "12345678901234567",
			expected:  []int{10, 10}, // Second block padded
		},
		{
			name:      "multiple blocks exact",
			blockSize: 10,
			input:     "1234567890123456789012345",
			expected:  []int{10, 10, 10}, // All blocks exact, last needs padding
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			splitter, err := NewSplitter(tt.blockSize)
			if err != nil {
				t.Fatalf("Failed to create splitter: %v", err)
			}

			// Test splitting (always padded)
			blocks, err := splitter.Split(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("Split failed: %v", err)
			}

			if len(blocks) != len(tt.expected) {
				t.Fatalf("Expected %d blocks, got %d", len(tt.expected), len(blocks))
			}

			for i, block := range blocks {
				if block.Size() != tt.expected[i] {
					t.Errorf("Block %d: expected size %d, got %d", i, tt.expected[i], block.Size())
				}
			}

			// Verify the data is preserved (first part should match original)
			var result bytes.Buffer
			for _, block := range blocks {
				result.Write(block.Data)
			}
			
			// Check that the original data is preserved
			resultData := result.Bytes()
			if !bytes.HasPrefix(resultData, []byte(tt.input)) {
				t.Errorf("Original data not preserved in padded blocks")
			}
		})
	}
}

func TestSplitBytesWithPadding(t *testing.T) {
	blockSize := 8
	input := []byte("12345")
	
	splitter, err := NewSplitter(blockSize)
	if err != nil {
		t.Fatalf("Failed to create splitter: %v", err)
	}

	blocks, err := splitter.SplitBytes(input)
	if err != nil {
		t.Fatalf("SplitBytes failed: %v", err)
	}

	if len(blocks) != 1 {
		t.Fatalf("Expected 1 block, got %d", len(blocks))
	}

	block := blocks[0]
	if block.Size() != blockSize {
		t.Errorf("Expected block size %d, got %d", blockSize, block.Size())
	}

	// Verify original data is preserved
	if !bytes.HasPrefix(block.Data, input) {
		t.Errorf("Original data not preserved in padded block")
	}

	// Verify padding is zeros
	paddingStart := len(input)
	for i := paddingStart; i < len(block.Data); i++ {
		if block.Data[i] != 0 {
			t.Errorf("Padding byte at position %d should be 0, got %d", i, block.Data[i])
		}
	}
}

func TestPaddingRoundTrip(t *testing.T) {
	blockSize := 10
	originalData := "Hello, World!"
	
	splitter, err := NewSplitter(blockSize)
	if err != nil {
		t.Fatalf("Failed to create splitter: %v", err)
	}

	// Split with padding
	blocks, err := splitter.Split(strings.NewReader(originalData))
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	// Reassemble
	assembler := NewAssembler()
	var result bytes.Buffer
	if err := assembler.AssembleToWriter(blocks, &result); err != nil {
		t.Fatalf("AssembleToWriter failed: %v", err)
	}

	// Trim to original size (simulating what download does)
	reassembled := result.Bytes()[:len(originalData)]
	
	if string(reassembled) != originalData {
		t.Errorf("Round trip failed: expected %q, got %q", originalData, string(reassembled))
	}
}

func TestPaddingWithEmptyData(t *testing.T) {
	blockSize := 10
	splitter, err := NewSplitter(blockSize)
	if err != nil {
		t.Fatalf("Failed to create splitter: %v", err)
	}

	// Test with empty reader
	blocks, err := splitter.Split(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Split with empty data failed: %v", err)
	}

	if len(blocks) != 0 {
		t.Errorf("Expected 0 blocks for empty data, got %d", len(blocks))
	}
}

func TestPaddingConsistency(t *testing.T) {
	blockSize := 128
	testData := "This is test data that needs padding"
	
	splitter, err := NewSplitter(blockSize)
	if err != nil {
		t.Fatalf("Failed to create splitter: %v", err)
	}

	// Split the same data multiple times
	blocks1, err := splitter.Split(strings.NewReader(testData))
	if err != nil {
		t.Fatalf("First split failed: %v", err)
	}

	blocks2, err := splitter.Split(strings.NewReader(testData))
	if err != nil {
		t.Fatalf("Second split failed: %v", err)
	}

	// Verify consistency
	if len(blocks1) != len(blocks2) {
		t.Errorf("Inconsistent block count: %d vs %d", len(blocks1), len(blocks2))
	}

	for i := 0; i < len(blocks1) && i < len(blocks2); i++ {
		if !bytes.Equal(blocks1[i].Data, blocks2[i].Data) {
			t.Errorf("Block %d inconsistent between splits", i)
		}
	}
}