package blocks

import (
	"bytes"
	"strings"
	"testing"
)

// TestStreamingSplitter tests the basic streaming splitter functionality
func TestStreamingSplitter(t *testing.T) {
	testData := "Hello, World! This is a test of streaming splitting functionality."
	reader := strings.NewReader(testData)
	
	splitter, err := NewStreamingSplitter(16)
	if err != nil {
		t.Fatalf("Failed to create streaming splitter: %v", err)
	}
	
	var blocks []*Block
	processor := &testBlockProcessor{blocks: &blocks}
	
	err = splitter.Split(reader, processor)
	if err != nil {
		t.Fatalf("Failed to split data: %v", err)
	}
	
	expectedBlocks := (len(testData) + 15) / 16
	if len(blocks) != expectedBlocks {
		t.Errorf("Expected %d blocks, got %d", expectedBlocks, len(blocks))
	}
	
	var result bytes.Buffer
	for _, block := range blocks {
		result.Write(block.Data)
	}
	
	if result.String() != testData {
		t.Errorf("Reassembled data doesn't match original")
	}
}

// TestStreamingAssembler tests the streaming assembler functionality
func TestStreamingAssembler(t *testing.T) {
	var output bytes.Buffer
	
	assembler, err := NewStreamingAssembler(&output)
	if err != nil {
		t.Fatalf("Failed to create streaming assembler: %v", err)
	}
	
	assembler.SetTotalBlocks(3)
	
	block0, _ := NewBlock([]byte("Hello, "))
	block1, _ := NewBlock([]byte("World! "))
	block2, _ := NewBlock([]byte("Testing."))
	
	// Add blocks out of order
	err = assembler.AddBlock(1, block1)
	if err != nil {
		t.Fatalf("Failed to add block 1: %v", err)
	}
	
	err = assembler.AddBlock(2, block2)
	if err != nil {
		t.Fatalf("Failed to add block 2: %v", err)
	}
	
	if output.Len() > 0 {
		t.Errorf("Expected no output yet, got: %s", output.String())
	}
	
	// Add block 0 - should trigger writing of blocks 0, 1, 2
	err = assembler.AddBlock(0, block0)
	if err != nil {
		t.Fatalf("Failed to add block 0: %v", err)
	}
	
	expected := "Hello, World! Testing."
	if output.String() != expected {
		t.Errorf("Expected '%s', got '%s'", expected, output.String())
	}
	
	if !assembler.IsComplete() {
		t.Errorf("Expected assembler to be complete")
	}
}

// TestStreamingXOR tests the XOR processing functionality  
func TestStreamingXOR(t *testing.T) {
	dataBlock, _ := NewBlock([]byte("Hello World!"))
	randomizer1, _ := NewRandomBlock(12)
	randomizer2, _ := NewRandomBlock(12)
	
	provider, err := NewSimpleRandomizerProvider(randomizer1, randomizer2)
	if err != nil {
		t.Fatalf("Failed to create randomizer provider: %v", err)
	}
	
	var xorBlocks []*Block
	xorProcessor := &testBlockProcessor{blocks: &xorBlocks}
	
	streamingXOR, err := NewStreamingXORProcessor(provider, xorProcessor)
	if err != nil {
		t.Fatalf("Failed to create streaming XOR processor: %v", err)
	}
	
	err = streamingXOR.ProcessBlock(0, dataBlock)
	if err != nil {
		t.Fatalf("Failed to process block: %v", err)
	}
	
	if len(xorBlocks) != 1 {
		t.Fatalf("Expected 1 XOR block, got %d", len(xorBlocks))
	}
	
	// Verify XOR operation worked
	originalXOR, err := dataBlock.XOR(randomizer1, randomizer2)
	if err != nil {
		t.Fatalf("Failed to compute expected XOR: %v", err)
	}
	
	if !bytes.Equal(xorBlocks[0].Data, originalXOR.Data) {
		t.Errorf("XOR result doesn't match expected")
	}
}

// Test helper type
type testBlockProcessor struct {
	blocks *[]*Block
}

func (p *testBlockProcessor) ProcessBlock(blockIndex int, block *Block) error {
	*p.blocks = append(*p.blocks, block)
	return nil
}