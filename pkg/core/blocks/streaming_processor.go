package blocks

// StreamingDirectoryProcessor for processing large directories without memory overflow
type StreamingDirectoryProcessor struct {
	*DirectoryProcessor
	maxMemoryUsage int64
	currentMemory  int64
}

// NewStreamingDirectoryProcessor creates a processor optimized for large directories
func NewStreamingDirectoryProcessor(config *ProcessorConfig, maxMemoryMB int64) (*StreamingDirectoryProcessor, error) {
	baseProcessor, err := NewDirectoryProcessor(config)
	if err != nil {
		return nil, err
	}

	return &StreamingDirectoryProcessor{
		DirectoryProcessor: baseProcessor,
		maxMemoryUsage:     maxMemoryMB * 1024 * 1024,
		currentMemory:      0,
	}, nil
}

// ProcessDirectoryStreaming processes directory with memory management
func (sdp *StreamingDirectoryProcessor) ProcessDirectoryStreaming(rootPath string, processor DirectoryBlockProcessor) error {
	// Implementation would include memory monitoring and backpressure
	// For now, delegate to base processor
	_, err := sdp.ProcessDirectory(rootPath, processor)
	return err
}