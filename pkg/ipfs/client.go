package ipfs

import (
	"bytes"
	"context"
	"io"

	shell "github.com/ipfs/go-ipfs-api"
)

// Client interface for IPFS operations
type Client interface {
	// Get retrieves data from IPFS by CID
	Get(ctx context.Context, cid string) ([]byte, error)
	
	// Add stores data in IPFS and returns the CID
	Add(ctx context.Context, data []byte) (string, error)
	
	// Cat streams data from IPFS
	Cat(ctx context.Context, cid string) (io.ReadCloser, error)
	
	// IsConnected checks if the client is connected to IPFS
	IsConnected() bool
}

// ShellClient wraps the IPFS shell API
type ShellClient struct {
	shell *shell.Shell
}

// NewShellClient creates a new IPFS shell client
func NewShellClient(url string) Client {
	return &ShellClient{
		shell: shell.NewShell(url),
	}
}

// Get retrieves data from IPFS by CID
func (c *ShellClient) Get(ctx context.Context, cid string) ([]byte, error) {
	reader, err := c.shell.Cat(cid)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	
	return io.ReadAll(reader)
}

// Add stores data in IPFS and returns the CID
func (c *ShellClient) Add(ctx context.Context, data []byte) (string, error) {
	return c.shell.Add(bytes.NewReader(data))
}

// Cat streams data from IPFS
func (c *ShellClient) Cat(ctx context.Context, cid string) (io.ReadCloser, error) {
	return c.shell.Cat(cid)
}

// IsConnected checks if the client is connected to IPFS
func (c *ShellClient) IsConnected() bool {
	// Simple connectivity check
	_, err := c.shell.ID()
	return err == nil
}