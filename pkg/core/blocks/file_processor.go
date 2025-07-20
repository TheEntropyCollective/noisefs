package blocks

import (
	"sync"

	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
)

// FileBlockProcessor wraps a DirectoryBlockProcessor for processing file blocks
type FileBlockProcessor struct {
	FilePath      string
	FileSize      int64
	Processor     DirectoryBlockProcessor
	Results       []*ProcessResult
	EncryptionKey *crypto.EncryptionKey
	fileCID       string
	mutex         sync.Mutex
}

// ProcessBlock implements BlockProcessor interface
func (fbp *FileBlockProcessor) ProcessBlock(blockIndex int, block *Block) error {
	fbp.mutex.Lock()
	defer fbp.mutex.Unlock()

	// Store the first block's CID as the file CID
	if blockIndex == 0 {
		fbp.fileCID = block.ID
	}

	return fbp.Processor.ProcessDirectoryBlock(blockIndex, block)
}

// GetFileCID returns the file's CID (first block's CID)
func (fbp *FileBlockProcessor) GetFileCID() string {
	fbp.mutex.Lock()
	defer fbp.mutex.Unlock()
	return fbp.fileCID
}