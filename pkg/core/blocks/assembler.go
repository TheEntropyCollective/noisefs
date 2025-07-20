// Package blocks provides file reconstruction functionality through the Assembler type.
// This file handles the reconstruction of original files from their anonymized blocks
// after XOR de-anonymization has been performed. The assembler supports both
// in-memory and streaming assembly modes for different use cases.
package blocks

import (
	"bytes"
	"errors"
	"io"
)

// Assembler handles reconstruction of files from de-anonymized blocks in their original order.
// This is the final step in the NoiseFS file retrieval process, occurring after blocks
// have been retrieved from storage and de-anonymized through XOR operations.
//
// The assembler is stateless and can be reused for multiple file reconstruction operations.
// It provides two assembly modes:
//   - In-memory assembly: Concatenates all blocks into a single byte slice
//   - Streaming assembly: Writes blocks directly to an io.Writer for memory efficiency
//
// Call Flow:
//   - Called by: Client.Download, Client.StreamingDownload
//   - Calls: bytes.Buffer.Write, io.Writer.Write
//
// Time Complexity: O(n) where n is the total size of all blocks
// Space Complexity: O(n) for in-memory assembly, O(1) for streaming assembly
type Assembler struct{}

// NewAssembler creates a new file assembler instance.
// The assembler is stateless, so a single instance can be reused across multiple operations.
//
// Returns:
//   - *Assembler: A new assembler instance ready for file reconstruction
//
// Call Flow:
//   - Called by: Client download methods, test functions
//   - Calls: None (simple constructor)
//
// Time Complexity: O(1)
// Space Complexity: O(1)
func NewAssembler() *Assembler {
	return &Assembler{}
}

// Assemble reconstructs a complete file from an ordered slice of de-anonymized blocks.
// This method performs in-memory assembly, concatenating all block data into a single
// byte slice. The caller is responsible for trimming any padding from the final result.
//
// The method expects blocks to be provided in the correct order (index 0, 1, 2, ...).
// Each block should contain de-anonymized data (original content after XOR reversal).
// All blocks must be non-nil and contain valid data.
//
// Parameters:
//   - blocks: Ordered slice of de-anonymized blocks containing original file data
//
// Returns:
//   - []byte: Complete file data as a continuous byte slice
//   - error: Non-nil if assembly fails due to nil blocks or write errors
//
// Call Flow:
//   - Called by: Client.Download, Client.DownloadWithProgress
//   - Calls: bytes.Buffer.Write
//
// Time Complexity: O(n) where n is the total size of all blocks
// Space Complexity: O(n) - creates a complete copy of file data in memory
func (a *Assembler) Assemble(blocks []*Block) ([]byte, error) {
	if len(blocks) == 0 {
		return []byte{}, nil // Empty file case - return empty slice
	}

	// Use bytes.Buffer for efficient concatenation of variable-length blocks
	var buffer bytes.Buffer

	// Iterate through blocks in order, concatenating their data
	for _, block := range blocks {
		if block == nil {
			return nil, errors.New("nil block in assembly")
		}

		// Write block data to buffer - this should not fail for bytes.Buffer
		if _, err := buffer.Write(block.Data); err != nil {
			return nil, err
		}
	}

	return buffer.Bytes(), nil
}

// AssembleToWriter reconstructs a file from blocks and streams output directly to a writer.
// This method provides memory-efficient assembly for large files by avoiding buffering
// the entire file content in memory. Each block is written immediately to the provided writer.
//
// The method expects blocks to be provided in the correct order (index 0, 1, 2, ...).
// Each block should contain de-anonymized data (original content after XOR reversal).
// The writer should handle any necessary padding removal or size limiting.
//
// Parameters:
//   - blocks: Ordered slice of de-anonymized blocks containing original file data
//   - writer: Destination for reconstructed file data (must be non-nil)
//
// Returns:
//   - error: Non-nil if assembly fails due to nil blocks, nil writer, or write errors
//
// Call Flow:
//   - Called by: Client.StreamingDownload, streaming assembly operations
//   - Calls: io.Writer.Write
//
// Time Complexity: O(n) where n is the total size of all blocks
// Space Complexity: O(1) - streams data without buffering entire file
func (a *Assembler) AssembleToWriter(blocks []*Block, writer io.Writer) error {
	if len(blocks) == 0 {
		return nil // Empty file - nothing to write, successful operation
	}

	if writer == nil {
		return errors.New("writer cannot be nil")
	}

	// Stream each block directly to the writer without intermediate buffering
	for _, block := range blocks {
		if block == nil {
			return errors.New("nil block in assembly")
		}

		// Write block data directly to output - error handling depends on writer implementation
		if _, err := writer.Write(block.Data); err != nil {
			return err
		}
	}

	return nil
}
