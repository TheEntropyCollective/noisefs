package ipfs

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	shell "github.com/ipfs/go-ipfs-api"
	"github.com/TheEntropyCollective/noisefs/pkg/blocks"
)

// BlockStore defines the interface for IPFS block operations
type BlockStore interface {
	StoreBlock(block *blocks.Block) (string, error)
	RetrieveBlock(cid string) (*blocks.Block, error)
}

// Client handles interaction with IPFS
type Client struct {
	shell *shell.Shell
}

// NewClient creates a new IPFS client
func NewClient(apiURL string) (*Client, error) {
	if apiURL == "" {
		apiURL = "localhost:5001" // Default IPFS API endpoint
	}
	
	sh := shell.NewShell(apiURL)
	
	// Test connection
	if _, err := sh.ID(); err != nil {
		return nil, fmt.Errorf("failed to connect to IPFS: %w", err)
	}
	
	return &Client{
		shell: sh,
	}, nil
}

// StoreBlock stores a block in IPFS and returns its CID
func (c *Client) StoreBlock(block *blocks.Block) (string, error) {
	if block == nil {
		return "", errors.New("block cannot be nil")
	}
	
	reader := bytes.NewReader(block.Data)
	cid, err := c.shell.Add(reader)
	if err != nil {
		return "", fmt.Errorf("failed to store block: %w", err)
	}
	
	return cid, nil
}

// RetrieveBlock retrieves a block from IPFS by its CID
func (c *Client) RetrieveBlock(cid string) (*blocks.Block, error) {
	if cid == "" {
		return nil, errors.New("CID cannot be empty")
	}
	
	reader, err := c.shell.Cat(cid)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve block: %w", err)
	}
	defer reader.Close()
	
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read block data: %w", err)
	}
	
	return blocks.NewBlock(data)
}

// StoreBlocks stores multiple blocks in IPFS and returns their CIDs
func (c *Client) StoreBlocks(blks []*blocks.Block) ([]string, error) {
	if len(blks) == 0 {
		return nil, errors.New("no blocks to store")
	}
	
	cids := make([]string, len(blks))
	
	for i, block := range blks {
		cid, err := c.StoreBlock(block)
		if err != nil {
			return nil, fmt.Errorf("failed to store block %d: %w", i, err)
		}
		cids[i] = cid
	}
	
	return cids, nil
}

// RetrieveBlocks retrieves multiple blocks from IPFS by their CIDs
func (c *Client) RetrieveBlocks(cids []string) ([]*blocks.Block, error) {
	if len(cids) == 0 {
		return nil, errors.New("no CIDs provided")
	}
	
	blks := make([]*blocks.Block, len(cids))
	
	for i, cid := range cids {
		block, err := c.RetrieveBlock(cid)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve block %d: %w", i, err)
		}
		blks[i] = block
	}
	
	return blks, nil
}

// PinBlock pins a block in IPFS to prevent garbage collection
func (c *Client) PinBlock(cid string) error {
	if cid == "" {
		return errors.New("CID cannot be empty")
	}
	
	return c.shell.Pin(cid)
}

// UnpinBlock unpins a block in IPFS
func (c *Client) UnpinBlock(cid string) error {
	if cid == "" {
		return errors.New("CID cannot be empty")
	}
	
	return c.shell.Unpin(cid)
}

// Add stores data in IPFS and returns its CID
func (c *Client) Add(reader io.Reader) (string, error) {
	if reader == nil {
		return "", errors.New("reader cannot be nil")
	}
	
	return c.shell.Add(reader)
}

// Cat retrieves data from IPFS by its CID
func (c *Client) Cat(cid string) (io.ReadCloser, error) {
	if cid == "" {
		return nil, errors.New("CID cannot be empty")
	}
	
	return c.shell.Cat(cid)
}