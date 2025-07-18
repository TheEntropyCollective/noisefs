package blocks

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestPaddingIntegration(t *testing.T) {
	// Test various file sizes to ensure padding works correctly
	testCases := []struct {
		name      string
		content   string
		blockSize int
	}{
		{
			name:      "small file needing padding",
			content:   "Hello, World!",
			blockSize: 32,
		},
		{
			name:      "file exactly block size",
			content:   "This is exactly 32 bytes long!!",
			blockSize: 32,
		},
		{
			name:      "file larger than block size",
			content:   "This is a longer file that will span multiple blocks and the last block will need padding",
			blockSize: 32,
		},
		{
			name:      "very small file",
			content:   "Hi",
			blockSize: 128,
		},
		{
			name:      "large file with default block size",
			content:   strings.Repeat("Large file content. ", 1000), // ~20KB
			blockSize: DefaultBlockSize,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create splitter
			splitter, err := NewSplitter(tc.blockSize)
			if err != nil {
				t.Fatalf("Failed to create splitter: %v", err)
			}

			// Split with padding
			blocks, err := splitter.SplitWithPadding(strings.NewReader(tc.content))
			if err != nil {
				t.Fatalf("Failed to split with padding: %v", err)
			}

			// Verify all blocks are consistent size (except possibly empty case)
			if len(blocks) > 0 {
				for i, block := range blocks {
					if block.Size() != tc.blockSize {
						t.Errorf("Block %d has size %d, expected %d", i, block.Size(), tc.blockSize)
					}
				}
			}

			// Assemble blocks
			assembler := NewAssembler()
			var result bytes.Buffer
			if err := assembler.AssembleToWriter(blocks, &result); err != nil {
				t.Fatalf("Failed to assemble blocks: %v", err)
			}

			// Verify that the original content is preserved (trim to original size)
			assembledData := result.Bytes()
			originalData := []byte(tc.content)
			
			if len(assembledData) < len(originalData) {
				t.Fatalf("Assembled data is shorter than original: got %d, expected at least %d", 
					len(assembledData), len(originalData))
			}

			// Check that original data is preserved
			if !bytes.Equal(assembledData[:len(originalData)], originalData) {
				t.Errorf("Original data not preserved after padding roundtrip")
				t.Errorf("Expected: %q", string(originalData))
				t.Errorf("Got:      %q", string(assembledData[:len(originalData)]))
			}

			// If there was padding, verify padding bytes are zero
			if len(assembledData) > len(originalData) {
				paddingBytes := assembledData[len(originalData):]
				for i, b := range paddingBytes {
					if b != 0 {
						t.Errorf("Padding byte at offset %d should be 0, got %d", i, b)
					}
				}
			}

			// Calculate storage efficiency
			originalSize := len(tc.content)
			paddedSize := len(blocks) * tc.blockSize
			
			if originalSize > 0 {
				overhead := float64(paddedSize-originalSize) / float64(originalSize) * 100
				t.Logf("Storage efficiency: %d bytes → %d bytes (%.1f%% overhead)", 
					originalSize, paddedSize, overhead)
			}
		})
	}
}

func TestPaddingConsistencyAcrossOperations(t *testing.T) {
	content := "This is test content that will be padded"
	blockSize := 32

	splitter, err := NewSplitter(blockSize)
	if err != nil {
		t.Fatalf("Failed to create splitter: %v", err)
	}

	// Split using both methods
	blocks1, err := splitter.SplitWithPadding(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Failed to split with padding (method 1): %v", err)
	}

	blocks2, err := splitter.SplitBytesWithPadding([]byte(content))
	if err != nil {
		t.Fatalf("Failed to split with padding (method 2): %v", err)
	}

	// Verify both methods produce identical results
	if len(blocks1) != len(blocks2) {
		t.Fatalf("Different number of blocks: %d vs %d", len(blocks1), len(blocks2))
	}

	for i := 0; i < len(blocks1); i++ {
		if !bytes.Equal(blocks1[i].Data, blocks2[i].Data) {
			t.Errorf("Block %d differs between methods", i)
		}
	}
}

func TestPaddingWithVariousBlockSizes(t *testing.T) {
	content := "This is a test file with some content that will be used to test padding behavior"
	blockSizes := []int{16, 32, 64, 128, 256, 512, 1024}

	for _, blockSize := range blockSizes {
		t.Run(fmt.Sprintf("blockSize_%d", blockSize), func(t *testing.T) {
			splitter, err := NewSplitter(blockSize)
			if err != nil {
				t.Fatalf("Failed to create splitter with size %d: %v", blockSize, err)
			}

			blocks, err := splitter.SplitWithPadding(strings.NewReader(content))
			if err != nil {
				t.Fatalf("Failed to split with padding: %v", err)
			}

			// Verify all blocks have consistent size
			for i, block := range blocks {
				if block.Size() != blockSize {
					t.Errorf("Block %d has size %d, expected %d", i, block.Size(), blockSize)
				}
			}

			// Verify data integrity
			assembler := NewAssembler()
			var result bytes.Buffer
			if err := assembler.AssembleToWriter(blocks, &result); err != nil {
				t.Fatalf("Failed to assemble: %v", err)
			}

			// Check original content is preserved
			assembledData := result.Bytes()
			originalData := []byte(content)
			
			if !bytes.Equal(assembledData[:len(originalData)], originalData) {
				t.Errorf("Data integrity check failed for block size %d", blockSize)
			}
		})
	}
}