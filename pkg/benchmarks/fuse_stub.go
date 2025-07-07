// +build !fuse

package benchmarks

import (
	"fmt"

	"github.com/TheEntropyCollective/noisefs/pkg/logging"
)

// FUSEBenchmarkSuite provides benchmarks specifically for FUSE operations
type FUSEBenchmarkSuite struct {
	*BenchmarkSuite
	mountPath string
}

// NewFUSEBenchmarkSuite creates a new FUSE benchmark suite (stub for non-FUSE builds)
func NewFUSEBenchmarkSuite(mountPath string, logger *logging.Logger) *FUSEBenchmarkSuite {
	return &FUSEBenchmarkSuite{
		BenchmarkSuite: NewBenchmarkSuite("FUSE Operations (Disabled)", "", logger),
		mountPath:      mountPath,
	}
}

// BenchmarkFUSEFileOperations is a stub that returns an error
func (fbs *FUSEBenchmarkSuite) BenchmarkFUSEFileOperations(config *BenchmarkConfig) error {
	return fmt.Errorf("FUSE benchmarks not available (build without fuse tag)")
}

// BenchmarkFUSEMetadata is a stub that returns an error
func (fbs *FUSEBenchmarkSuite) BenchmarkFUSEMetadata(config *BenchmarkConfig) error {
	return fmt.Errorf("FUSE benchmarks not available (build without fuse tag)")
}

// BenchmarkFUSEDirectoryOps is a stub that returns an error
func (fbs *FUSEBenchmarkSuite) BenchmarkFUSEDirectoryOps(config *BenchmarkConfig) error {
	return fmt.Errorf("FUSE benchmarks not available (build without fuse tag)")
}

// RunFullFUSEBenchmarkSuite is a stub that returns an error
func (fbs *FUSEBenchmarkSuite) RunFullFUSEBenchmarkSuite(config *BenchmarkConfig) error {
	return fmt.Errorf("FUSE benchmarks not available (build without fuse tag)")
}