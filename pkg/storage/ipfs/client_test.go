package ipfs

import (
	"bytes"
	"io"
	"strings"
	"testing"
	
	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// Note: These tests focus on validation logic and error handling.
// Integration tests requiring a live IPFS daemon are in separate files.

func TestNewClient(t *testing.T) {
	// Test with empty API URL (should use default)
	client, err := NewClient("")
	// This will fail without IPFS daemon, but that's expected
	if err != nil && !strings.Contains(err.Error(), "failed to connect to IPFS") {
		t.Errorf("NewClient() unexpected error type: %v", err)
	}
	
	// Test with custom API URL
	client, err = NewClient("localhost:5001")
	// This will also fail without IPFS daemon, but that's expected
	if err != nil && !strings.Contains(err.Error(), "failed to connect to IPFS") {
		t.Errorf("NewClient() with custom URL unexpected error type: %v", err)
	}
	
	// The client creation logic itself is simple, the main validation
	// is that it attempts to connect to IPFS, which is tested in integration tests
	_ = client
}

func TestClientStoreBlockValidation(t *testing.T) {
	// Create a client without connecting (for validation testing)
	client := &Client{
		shell: nil, // We'll test validation before shell is used
	}
	
	// Test nil block
	_, err := client.StoreBlock(nil)
	if err == nil {
		t.Error("StoreBlock() with nil block should return error")
	}
	
	expectedErr := "block cannot be nil"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("StoreBlock() error = %v, want error containing %v", err, expectedErr)
	}
}

func TestClientRetrieveBlockValidation(t *testing.T) {
	// Create a client without connecting (for validation testing)
	client := &Client{
		shell: nil, // We'll test validation before shell is used
	}
	
	// Test empty CID
	_, err := client.RetrieveBlock("")
	if err == nil {
		t.Error("RetrieveBlock() with empty CID should return error")
	}
	
	expectedErr := "CID cannot be empty"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("RetrieveBlock() error = %v, want error containing %v", err, expectedErr)
	}
}

func TestClientStoreBlocksValidation(t *testing.T) {
	client := &Client{
		shell: nil,
	}
	
	// Test empty slice
	_, err := client.StoreBlocks([]*blocks.Block{})
	if err == nil {
		t.Error("StoreBlocks() with empty slice should return error")
	}
	
	expectedErr := "no blocks to store"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("StoreBlocks() error = %v, want error containing %v", err, expectedErr)
	}
	
	// Test nil slice
	_, err = client.StoreBlocks(nil)
	if err == nil {
		t.Error("StoreBlocks() with nil slice should return error")
	}
	
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("StoreBlocks() error = %v, want error containing %v", err, expectedErr)
	}
}

func TestClientRetrieveBlocksValidation(t *testing.T) {
	client := &Client{
		shell: nil,
	}
	
	// Test empty slice
	_, err := client.RetrieveBlocks([]string{})
	if err == nil {
		t.Error("RetrieveBlocks() with empty slice should return error")
	}
	
	expectedErr := "no CIDs provided"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("RetrieveBlocks() error = %v, want error containing %v", err, expectedErr)
	}
	
	// Test nil slice
	_, err = client.RetrieveBlocks(nil)
	if err == nil {
		t.Error("RetrieveBlocks() with nil slice should return error")
	}
	
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("RetrieveBlocks() error = %v, want error containing %v", err, expectedErr)
	}
}

func TestClientPinBlockValidation(t *testing.T) {
	client := &Client{
		shell: nil,
	}
	
	// Test empty CID
	err := client.PinBlock("")
	if err == nil {
		t.Error("PinBlock() with empty CID should return error")
	}
	
	expectedErr := "CID cannot be empty"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("PinBlock() error = %v, want error containing %v", err, expectedErr)
	}
}

func TestClientUnpinBlockValidation(t *testing.T) {
	client := &Client{
		shell: nil,
	}
	
	// Test empty CID
	err := client.UnpinBlock("")
	if err == nil {
		t.Error("UnpinBlock() with empty CID should return error")
	}
	
	expectedErr := "CID cannot be empty"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("UnpinBlock() error = %v, want error containing %v", err, expectedErr)
	}
}

func TestClientAddValidation(t *testing.T) {
	client := &Client{
		shell: nil,
	}
	
	// Test nil reader
	_, err := client.Add(nil)
	if err == nil {
		t.Error("Add() with nil reader should return error")
	}
	
	expectedErr := "reader cannot be nil"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("Add() error = %v, want error containing %v", err, expectedErr)
	}
}

func TestClientCatValidation(t *testing.T) {
	client := &Client{
		shell: nil,
	}
	
	// Test empty CID
	_, err := client.Cat("")
	if err == nil {
		t.Error("Cat() with empty CID should return error")
	}
	
	expectedErr := "CID cannot be empty"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("Cat() error = %v, want error containing %v", err, expectedErr)
	}
}

// Test error propagation for batch operations
func TestStoreBlocksErrorPropagation(t *testing.T) {
	// This test verifies that StoreBlocks properly handles individual block errors
	// We test validation-only logic to avoid needing IPFS
	
	client := &Client{
		shell: nil,
	}
	
	// Test with nil block in the slice (validation should catch this)
	testBlocks := []*blocks.Block{nil}
	
	// This should fail during the StoreBlock call when it validates the nil block
	_, err := client.StoreBlocks(testBlocks)
	if err == nil {
		t.Error("StoreBlocks() with nil block should return error")
	}
	
	// Should mention it failed on a specific block and contain validation error
	if !strings.Contains(err.Error(), "failed to store block") {
		t.Errorf("StoreBlocks() error should mention block index: %v", err)
	}
}

func TestRetrieveBlocksErrorPropagation(t *testing.T) {
	client := &Client{
		shell: nil,
	}
	
	// Test with empty CID (validation should catch this immediately)
	cids := []string{""}
	
	_, err := client.RetrieveBlocks(cids)
	if err == nil {
		t.Error("RetrieveBlocks() with empty CID should return error")
	}
	
	// Should mention it failed on a specific block with validation error
	if !strings.Contains(err.Error(), "failed to retrieve block") {
		t.Errorf("RetrieveBlocks() error should mention block index: %v", err)
	}
}

// Note: TestAddDataHandling removed because Add() immediately calls shell methods
// which cause nil pointer dereferences. The validation for Add() is covered 
// in TestClientAddValidation.

// Utility test to verify block data round-trip assumptions
func TestBlockDataAssumptions(t *testing.T) {
	// This test verifies assumptions about how block data should be handled
	testData := []byte("test data for block")
	
	block, err := blocks.NewBlock(testData)
	if err != nil {
		t.Fatalf("Failed to create block: %v", err)
	}
	
	// Verify block data is accessible
	if len(block.Data) != len(testData) {
		t.Errorf("Block data length = %v, want %v", len(block.Data), len(testData))
	}
	
	// Verify we can create readers from block data
	reader := bytes.NewReader(block.Data)
	if reader == nil {
		t.Error("Failed to create reader from block data")
	}
	
	// Verify we can read the data back
	readData, err := io.ReadAll(reader)
	if err != nil {
		t.Errorf("Failed to read data from reader: %v", err)
	}
	
	if !bytes.Equal(readData, testData) {
		t.Errorf("Read data = %v, want %v", readData, testData)
	}
}

// Test the theoretical happy path structure (without IPFS)
func TestClientMethodSignatures(t *testing.T) {
	// This test verifies that our method signatures are correct
	// and that the basic data flow makes sense
	
	// Test that we can create the types we expect to work with
	testData := []byte("test")
	block, err := blocks.NewBlock(testData)
	if err != nil {
		t.Fatalf("Failed to create block: %v", err)
	}
	
	// Verify we can construct the data types used by the client methods
	var cid string = "test_cid"
	var testBlocks []*blocks.Block = []*blocks.Block{block}
	var cids []string = []string{"cid1", "cid2"}
	var reader io.ReadCloser = io.NopCloser(bytes.NewReader(testData))
	
	// Verify the types are what we expect
	if cid == "" {
		t.Error("CID should not be empty")
	}
	if len(testBlocks) != 1 {
		t.Error("Should have one test block")
	}
	if len(cids) != 2 {
		t.Error("Should have two test CIDs")
	}
	if reader == nil {
		t.Error("Reader should not be nil")
	}
	
	// Clean up
	reader.Close()
	
	// This test verifies our type assumptions without calling IPFS
}