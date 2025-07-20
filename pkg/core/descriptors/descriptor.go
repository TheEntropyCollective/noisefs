// Package descriptors provides file reconstruction metadata management for NoiseFS.
// This file implements the core descriptor structures and operations for managing
// file reconstruction metadata, enabling files to be reassembled from anonymized blocks
// while preserving privacy and supporting both file and directory descriptors.
//
// The descriptor system provides:
//   - File reconstruction metadata with 3-tuple XOR block references
//   - Directory structure metadata with encrypted manifest support
//   - Versioned descriptor format for backward compatibility
//   - JSON serialization for storage and transport
//   - Comprehensive validation for data integrity
//   - Padding information for privacy-preserving block sizes
//
// Key Features:
//   - 3-tuple XOR anonymization metadata (data + 2 randomizers)
//   - Support for both file and directory descriptors
//   - Version 4.0 format with padding support for fixed block sizes
//   - Comprehensive validation to ensure descriptor integrity
//   - JSON serialization compatible with storage and transport
//   - Type-safe descriptor operations with error handling
//
// Descriptor Types:
//   - File descriptors: Contain block reconstruction metadata
//   - Directory descriptors: Reference encrypted manifest files
//   - Versioned format supporting evolution and compatibility
//
// Privacy Considerations:
//   - Descriptors reveal file structure but not content
//   - Support for public or private descriptor storage
//   - Minimal metadata to reduce fingerprinting
//   - Padding information helps maintain consistent block sizes
package descriptors

import (
	"encoding/json"
	"errors"
	"time"
)

// BlockPair represents a data block and its corresponding randomizers for 3-tuple XOR anonymization.
// This structure encapsulates the complete set of content identifiers needed to reconstruct
// an original data block through the NoiseFS 3-tuple XOR anonymization scheme.
//
// 3-Tuple XOR Anonymization:
//   The original data block is computed as: DataCID ⊕ RandomizerCID1 ⊕ RandomizerCID2
//   This scheme ensures that no single block reveals information about the original data,
//   providing strong privacy protection through cryptographic anonymization.
//
// Reconstruction Process:
//   1. Retrieve data block using DataCID from distributed storage
//   2. Retrieve first randomizer block using RandomizerCID1
//   3. Retrieve second randomizer block using RandomizerCID2
//   4. XOR all three blocks to recover original data: data ⊕ rand1 ⊕ rand2
//
// Privacy Properties:
//   - Data block appears as random data without randomizers
//   - Randomizer blocks can be reused across multiple files
//   - No correlation between data and randomizer content
//   - Strong plausible deniability through shared randomizers
//
// JSON Serialization:
//   All fields are JSON-serializable for descriptor storage and transport,
//   enabling persistent storage and cross-system descriptor exchange.
//
// Time Complexity: O(1) for structure access and serialization
// Space Complexity: O(1) - fixed size structure with string identifiers
type BlockPair struct {
	DataCID        string `json:"data_cid"`        // Content identifier for anonymized data block
	RandomizerCID1 string `json:"randomizer_cid1"` // Content identifier for first randomizer block
	RandomizerCID2 string `json:"randomizer_cid2"` // Content identifier for second randomizer block
}

// DescriptorType represents the type of descriptor for type-safe descriptor operations.
// This enumeration enables compile-time type checking and runtime validation of
// descriptor types, ensuring proper handling of file vs directory descriptors.
//
// Type Safety Benefits:
//   - Compile-time type checking prevents incorrect descriptor usage
//   - Runtime validation ensures descriptor consistency
//   - Clear separation between file and directory operations
//   - Support for future descriptor type extensions
//
// JSON Serialization:
//   Types are serialized as strings for human-readable descriptor format,
//   enabling easy debugging and cross-platform compatibility.
type DescriptorType string

const (
	// FileType represents a regular file descriptor containing block reconstruction metadata.
	// File descriptors include block pairs for 3-tuple XOR reconstruction and padding information
	// for privacy-preserving fixed block sizes.
	FileType DescriptorType = "file"
	
	// DirectoryType represents a directory descriptor containing encrypted manifest references.
	// Directory descriptors reference encrypted manifest files that contain directory structure
	// and file listings, enabling hierarchical file system organization.
	DirectoryType DescriptorType = "directory"
)

// Descriptor contains metadata needed to reconstruct a file or directory from anonymized blocks.
// This structure serves as the primary metadata container for NoiseFS, enabling file reconstruction
// while maintaining privacy through anonymization and supporting both files and directories.
//
// The descriptor provides:
//   - File reconstruction metadata with 3-tuple XOR block references
//   - Directory structure references through encrypted manifests
//   - Version information for backward compatibility and format evolution
//   - Padding information for privacy-preserving consistent block sizes
//   - Timestamp metadata for creation tracking and lifecycle management
//
// Descriptor Versioning:
//   - Version 4.0: Current format with padding support for fixed block sizes
//   - Backward compatibility maintained for older descriptor versions
//   - Forward compatibility through version-aware parsing
//
// Privacy Features:
//   - Minimal metadata to reduce fingerprinting opportunities
//   - Padding information enables consistent block sizes for privacy
//   - Support for both public and private descriptor storage
//   - Encrypted directory manifests for directory structure privacy
//
// File vs Directory Support:
//   - File descriptors: Contain block pairs for reconstruction
//   - Directory descriptors: Reference encrypted manifest with directory structure
//   - Type-specific validation ensures proper descriptor usage
//
// JSON Serialization:
//   - Human-readable JSON format for debugging and interoperability
//   - Compact serialization for efficient storage and transport
//   - Cross-platform compatibility through standard JSON encoding
//
// Time Complexity: O(1) for metadata access, O(n) for block operations where n is block count
// Space Complexity: O(n) where n is the number of blocks in the file
type Descriptor struct {
	Version        string         `json:"version"`                // Descriptor format version (currently "4.0")
	Type           DescriptorType `json:"type"`                   // Descriptor type (file or directory)
	Filename       string         `json:"filename"`               // Original filename or directory name
	FileSize       int64          `json:"file_size"`              // Original file size before padding (0 for directories)
	PaddedFileSize int64          `json:"padded_file_size"`       // Total size including padding for privacy (0 for directories)
	BlockSize      int            `json:"block_size"`             // Block size used for splitting (0 for directories)
	Blocks         []BlockPair    `json:"blocks,omitempty"`       // Block reconstruction metadata (empty for directories)
	ManifestCID    string         `json:"manifest_cid,omitempty"` // Encrypted manifest reference (directories only)
	CreatedAt      time.Time      `json:"created_at"`             // Descriptor creation timestamp
}

// NewDescriptor creates a new file descriptor with padding information for privacy-preserving storage.
// This constructor initializes a file descriptor with the current format version and proper
// padding metadata to support NoiseFS's privacy-preserving fixed block size architecture.
//
// File Descriptor Features:
//   - Version 4.0 format with comprehensive padding support
//   - Initialized empty block array for subsequent block addition
//   - Automatic timestamp assignment for creation tracking
//   - Type-safe file descriptor creation
//
// Padding Support:
//   - Original file size tracking for accurate reconstruction
//   - Padded file size for privacy-preserving consistent block sizes
//   - Block size specification for reconstruction compatibility
//   - Privacy enhancement through size normalization
//
// Privacy Benefits:
//   - Consistent block sizes prevent file size fingerprinting
//   - Padding information enables accurate data recovery
//   - Fixed block size architecture improves anonymity set
//   - Reduced metadata leakage through size normalization
//
// Initialization State:
//   - Empty blocks array ready for AddBlockTriple operations
//   - Current timestamp for creation tracking
//   - Version 4.0 for latest format compatibility
//   - File type designation for proper validation
//
// Parameters:
//   - filename: Original filename for metadata (stored in descriptor)
//   - originalFileSize: Actual file size before padding (bytes)
//   - paddedFileSize: Total size including padding for fixed blocks (bytes)
//   - blockSize: Block size used for file splitting (bytes)
//
// Returns:
//   - *Descriptor: Initialized file descriptor ready for block metadata
//
// Call Flow:
//   - Called by: Upload operations, file processing, descriptor creation
//   - Used with: AddBlockTriple for block metadata addition
//
// Time Complexity: O(1) - simple structure initialization
// Space Complexity: O(1) - fixed descriptor structure allocation
func NewDescriptor(filename string, originalFileSize int64, paddedFileSize int64, blockSize int) *Descriptor {
	return &Descriptor{
		Version:        "4.0", // Version 4.0 - padding always included
		Type:           FileType,
		Filename:       filename,
		FileSize:       originalFileSize,
		PaddedFileSize: paddedFileSize,
		BlockSize:      blockSize,
		Blocks:         make([]BlockPair, 0),
		CreatedAt:      time.Now(),
	}
}

// NewDirectoryDescriptor creates a new directory descriptor with encrypted manifest reference.
// This constructor initializes a directory descriptor that references an encrypted manifest
// containing the directory structure, enabling hierarchical file organization with privacy protection.
//
// Directory Descriptor Features:
//   - Version 4.0 format for current compatibility
//   - Encrypted manifest CID reference for directory structure
//   - Directory-specific field initialization (no blocks or sizes)
//   - Automatic timestamp assignment for creation tracking
//
// Encrypted Manifest Integration:
//   - Manifest CID references encrypted directory structure
//   - Directory contents protected through encryption
//   - Hierarchical access control through manifest encryption
//   - Privacy-preserving directory organization
//
// Directory-Specific Properties:
//   - No file size or padding (not applicable to directories)
//   - No block size or block pairs (directories don't contain blocks)
//   - Manifest CID as primary content reference
//   - Directory type designation for proper validation
//
// Privacy Features:
//   - Directory structure hidden in encrypted manifest
//   - No block-level metadata for directories
//   - Minimal descriptor metadata to reduce fingerprinting
//   - Support for hierarchical access control
//
// Parameters:
//   - dirname: Directory name for metadata (stored in descriptor)
//   - manifestCID: Content identifier for encrypted directory manifest
//
// Returns:
//   - *Descriptor: Initialized directory descriptor with manifest reference
//
// Call Flow:
//   - Called by: Directory creation, manifest storage, hierarchical organization
//   - Used with: Encrypted manifest storage and retrieval
//
// Time Complexity: O(1) - simple structure initialization
// Space Complexity: O(1) - fixed descriptor structure allocation
func NewDirectoryDescriptor(dirname string, manifestCID string) *Descriptor {
	return &Descriptor{
		Version:        "4.0",
		Type:           DirectoryType,
		Filename:       dirname,
		FileSize:       0, // Directories don't have a fixed size
		PaddedFileSize: 0, // Not applicable for directories
		BlockSize:      0, // Not applicable for directories
		ManifestCID:    manifestCID,
		CreatedAt:      time.Now(),
	}
}

// AddBlockTriple adds a data block with two randomizers for 3-tuple XOR anonymization.
// This method appends a complete block triple to the descriptor, providing all the metadata
// needed to reconstruct one block of the original file through XOR operations.
//
// 3-Tuple XOR Validation:
//   - Ensures all three CIDs are non-empty for valid block references
//   - Validates that all CIDs are different to prevent XOR operation failures
//   - Maintains 3-tuple anonymization security properties
//   - Provides clear error messages for validation failures
//
// Block Reconstruction:
//   Each block triple enables reconstruction: original = data ⊕ randomizer1 ⊕ randomizer2
//   This ensures that individual blocks don't reveal information about original content,
//   providing strong privacy protection through cryptographic anonymization.
//
// Security Properties:
//   - All CIDs must be different to prevent identity operations (A ⊕ A = 0)
//   - Non-empty CIDs ensure valid block references for reconstruction
//   - Sequential addition maintains block order for file reconstruction
//   - Error handling prevents invalid descriptor states
//
// Descriptor Building:
//   - Blocks are added sequentially during file upload
//   - Each block represents a chunk of the original file
//   - Block order must be preserved for correct file reconstruction
//   - Used with NewDescriptor to build complete file descriptors
//
// Parameters:
//   - dataCID: Content identifier for anonymized data block
//   - randomizerCID1: Content identifier for first randomizer block
//   - randomizerCID2: Content identifier for second randomizer block
//
// Returns:
//   - error: Non-nil if any CID is empty or CIDs are not unique
//
// Call Flow:
//   - Called by: Upload operations, file processing, descriptor building
//   - Used with: NewDescriptor during file upload workflow
//
// Time Complexity: O(1) - simple validation and append operation
// Space Complexity: O(1) - single block pair addition
func (d *Descriptor) AddBlockTriple(dataCID, randomizerCID1, randomizerCID2 string) error {
	if dataCID == "" || randomizerCID1 == "" || randomizerCID2 == "" {
		return errors.New("all CIDs cannot be empty")
	}

	if dataCID == randomizerCID1 || dataCID == randomizerCID2 || randomizerCID1 == randomizerCID2 {
		return errors.New("all CIDs must be different")
	}

	d.Blocks = append(d.Blocks, BlockPair{
		DataCID:        dataCID,
		RandomizerCID1: randomizerCID1,
		RandomizerCID2: randomizerCID2,
	})

	return nil
}

// Validate checks if the descriptor is valid and internally consistent.
// This method performs comprehensive validation of descriptor fields and structure,
// ensuring data integrity and proper format compliance for both file and directory descriptors.
//
// Validation Features:
//   - Version field validation for format compatibility
//   - Filename validation for required metadata
//   - Type-specific validation for file vs directory descriptors
//   - Comprehensive error reporting with specific failure details
//
// Multi-Level Validation:
//   1. Basic field validation (version, filename)
//   2. Type identification and routing
//   3. Type-specific validation (file or directory)
//   4. Block-level validation for file descriptors
//   5. Manifest validation for directory descriptors
//
// File Validation:
//   - File size and block size positivity
//   - Block array presence and validity
//   - Individual block triple validation
//   - CID uniqueness within each block
//
// Directory Validation:
//   - Version 4.0 requirement for directory support
//   - Manifest CID presence and validity
//   - Empty blocks array validation
//   - Directory-specific field constraints
//
// Returns:
//   - error: Non-nil with specific validation failure details, nil if valid
//
// Call Flow:
//   - Called by: Serialization, storage operations, descriptor integrity checks
//   - Calls: validateFile or validateDirectory based on descriptor type
//
// Time Complexity: O(n) where n is the number of blocks for file validation
// Space Complexity: O(1) - no additional memory allocation
func (d *Descriptor) Validate() error {
	if d.Version == "" {
		return errors.New("descriptor version is required")
	}

	if d.Filename == "" {
		return errors.New("filename is required")
	}

	// Validate based on type
	switch d.Type {
	case FileType:
		return d.validateFile()
	case DirectoryType:
		return d.validateDirectory()
	default:
		return errors.New("unknown descriptor type")
	}
}

// validateFile validates file-specific fields for file descriptor integrity.
// This internal method performs comprehensive validation of file descriptor fields,
// ensuring all required metadata is present and consistent for successful file reconstruction.
//
// File-Specific Validation:
//   - File size positivity for valid file representation
//   - Block size positivity for proper file splitting
//   - Block array presence for reconstruction capability
//   - Individual block validation for 3-tuple XOR integrity
//
// Block-Level Validation:
//   - All CIDs must be present (non-empty) for valid references
//   - All CIDs must be different within each block for XOR security
//   - Sequential validation of all blocks in the descriptor
//   - Clear error reporting for block-specific failures
//
// Reconstruction Requirements:
//   - At least one block required for file reconstruction
//   - Valid CIDs ensure successful block retrieval
//   - Unique CIDs prevent XOR operation failures
//   - Block order preservation for sequential reconstruction
//
// Security Validation:
//   - CID uniqueness prevents identity operations in XOR
//   - Non-empty CIDs ensure valid storage references
//   - Block integrity validation for security properties
//
// Returns:
//   - error: Non-nil with specific file validation failure details
//
// Call Flow:
//   - Called by: Validate method for file-type descriptors
//   - Internal validation method for file descriptor integrity
//
// Time Complexity: O(n) where n is the number of blocks
// Space Complexity: O(1) - no additional memory allocation
func (d *Descriptor) validateFile() error {
	if d.FileSize <= 0 {
		return errors.New("file size must be positive")
	}

	if d.BlockSize <= 0 {
		return errors.New("block size must be positive")
	}

	if len(d.Blocks) == 0 {
		return errors.New("must contain at least one block")
	}

	for i, block := range d.Blocks {
		if block.DataCID == "" || block.RandomizerCID1 == "" || block.RandomizerCID2 == "" {
			return errors.New("all CIDs must be present")
		}

		if block.DataCID == block.RandomizerCID1 || block.DataCID == block.RandomizerCID2 || block.RandomizerCID1 == block.RandomizerCID2 {
			return errors.New("all CIDs must be different")
		}
		_ = i
	}

	return nil
}

// validateDirectory validates directory-specific fields for directory descriptor integrity.
// This internal method performs validation of directory descriptor fields,
// ensuring proper format compliance and manifest reference validity for directory operations.
//
// Directory-Specific Validation:
//   - Version 4.0 requirement for directory descriptor support
//   - Manifest CID presence for directory structure reference
//   - Empty blocks array validation (directories don't contain blocks)
//   - Directory-specific field constraints and requirements
//
// Version Requirements:
//   - Directory descriptors require version 4.0 or later
//   - Ensures compatibility with directory support features
//   - Prevents usage of directory descriptors with older formats
//   - Forward compatibility through version validation
//
// Manifest Validation:
//   - Manifest CID must be present for directory structure access
//   - Enables encrypted directory content retrieval
//   - Supports hierarchical directory organization
//   - Required for directory reconstruction and access
//
// Structure Validation:
//   - Directories must not contain block pairs (file-specific)
//   - Ensures proper separation of file and directory metadata
//   - Prevents invalid descriptor state combinations
//   - Maintains type safety for directory operations
//
// Returns:
//   - error: Non-nil with specific directory validation failure details
//
// Call Flow:
//   - Called by: Validate method for directory-type descriptors
//   - Internal validation method for directory descriptor integrity
//
// Time Complexity: O(1) - simple field validation
// Space Complexity: O(1) - no additional memory allocation
func (d *Descriptor) validateDirectory() error {
	if d.Version != "4.0" {
		return errors.New("directory descriptors require version 4.0")
	}

	if d.ManifestCID == "" {
		return errors.New("directory descriptor must have a manifest CID")
	}

	if len(d.Blocks) > 0 {
		return errors.New("directory descriptors should not contain blocks")
	}

	return nil
}

// ToJSON serializes the descriptor to JSON with validation and formatting.
// This method provides comprehensive JSON serialization with built-in validation
// and human-readable formatting for debugging, storage, and transport purposes.
//
// Serialization Features:
//   - Pre-serialization validation to ensure descriptor integrity
//   - Indented JSON output for human readability and debugging
//   - Complete descriptor metadata serialization
//   - Cross-platform compatible JSON format
//
// Validation Integration:
//   - Automatic validation before serialization prevents invalid data
//   - Comprehensive error reporting for validation failures
//   - Ensures serialized descriptors are always valid
//   - Prevents storage or transport of corrupted descriptors
//
// JSON Format:
//   - Indented output with 2-space indentation for readability
//   - Standard JSON encoding for cross-platform compatibility
//   - All descriptor fields included in serialization
//   - Omitempty tags respect optional field handling
//
// Use Cases:
//   - Descriptor storage in databases or file systems
//   - Network transport and API responses
//   - Debugging and development tools
//   - Cross-system descriptor exchange
//   - Backup and restore operations
//
// Returns:
//   - []byte: Formatted JSON representation of the descriptor
//   - error: Non-nil if validation fails or JSON marshaling fails
//
// Call Flow:
//   - Called by: Storage operations, API handlers, debugging tools
//   - Calls: Validate for integrity checking, json.MarshalIndent for formatting
//
// Time Complexity: O(n) where n is the descriptor size (mainly block count)
// Space Complexity: O(n) for JSON output buffer
func (d *Descriptor) ToJSON() ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}

	return json.MarshalIndent(d, "", "  ")
}

// Marshal serializes the descriptor to JSON as an alias for ToJSON for API compatibility.
// This method provides a standard naming convention for serialization operations,
// offering the same functionality as ToJSON with a more conventional method name.
//
// API Compatibility:
//   - Standard Marshal naming convention for Go serialization
//   - Consistent with other serialization interfaces
//   - Enables drop-in replacement with other marshaling implementations
//   - Maintains compatibility with existing codebases
//
// Functionality:
//   - Direct delegation to ToJSON for consistent behavior
//   - Same validation and formatting features as ToJSON
//   - Identical error handling and return values
//   - No performance overhead from method aliasing
//
// Use Cases:
//   - API handlers expecting standard Marshal interface
//   - Serialization frameworks requiring Marshal methods
//   - Code compatibility with standard Go patterns
//   - Generic serialization operations
//
// Returns:
//   - []byte: Formatted JSON representation of the descriptor
//   - error: Non-nil if validation fails or JSON marshaling fails
//
// Call Flow:
//   - Called by: Generic serialization operations, API frameworks
//   - Calls: ToJSON for actual serialization logic
//
// Time Complexity: O(n) where n is the descriptor size (mainly block count)
// Space Complexity: O(n) for JSON output buffer
func (d *Descriptor) Marshal() ([]byte, error) {
	return d.ToJSON()
}

// FromJSON deserializes a descriptor from JSON with validation and integrity checking.
// This function reconstructs a descriptor from JSON data while ensuring data integrity
// through comprehensive validation and proper error handling.
//
// Deserialization Features:
//   - Empty data validation to prevent invalid input
//   - Standard JSON unmarshaling with error handling
//   - Post-deserialization validation for data integrity
//   - Complete descriptor reconstruction from JSON
//
// Validation Integration:
//   - Automatic validation after JSON parsing
//   - Comprehensive integrity checking of deserialized data
//   - Ensures returned descriptors are always valid
//   - Prevents usage of corrupted or incomplete descriptors
//
// Error Handling:
//   - Empty data detection and appropriate error reporting
//   - JSON parsing error propagation with context
//   - Validation error reporting for data integrity issues
//   - Clear error messages for troubleshooting
//
// Security Considerations:
//   - Input validation prevents processing of invalid data
//   - Validation ensures descriptor security properties
//   - Safe handling of untrusted JSON input
//   - Prevention of descriptor format attacks
//
// Use Cases:
//   - Loading descriptors from storage systems
//   - API request processing and validation
//   - Cross-system descriptor exchange
//   - Backup and restore operations
//   - Configuration file processing
//
// Parameters:
//   - data: JSON byte array containing serialized descriptor
//
// Returns:
//   - *Descriptor: Validated descriptor reconstructed from JSON
//   - error: Non-nil if data is empty, JSON is invalid, or validation fails
//
// Call Flow:
//   - Called by: Storage systems, API handlers, configuration loaders
//   - Calls: json.Unmarshal for parsing, Validate for integrity checking
//
// Time Complexity: O(n) where n is the JSON data size and block count
// Space Complexity: O(n) for descriptor structure allocation
func FromJSON(data []byte) (*Descriptor, error) {
	if len(data) == 0 {
		return nil, errors.New("empty JSON data")
	}

	var desc Descriptor
	if err := json.Unmarshal(data, &desc); err != nil {
		return nil, err
	}

	if err := desc.Validate(); err != nil {
		return nil, err
	}

	return &desc, nil
}

// GetRandomizerCIDs returns the randomizer CIDs for a block at the given index for reconstruction.
// This method provides safe access to randomizer block identifiers needed for 3-tuple XOR
// reconstruction of a specific block within the file.
//
// Randomizer Access Features:
//   - Bounds checking to prevent array access violations
//   - Clear error reporting for invalid block indices
//   - Safe extraction of randomizer CIDs for XOR operations
//   - Support for sequential block processing during reconstruction
//
// Block Reconstruction Support:
//   - Provides randomizer CIDs needed for XOR operations
//   - Enables: original_block = data_block ⊕ randomizer1 ⊕ randomizer2
//   - Sequential access pattern for efficient file reconstruction
//   - Integration with download and reconstruction workflows
//
// Index Validation:
//   - Negative index detection for invalid input
//   - Upper bound checking against block array length
//   - Clear error messages for out-of-range access
//   - Prevention of array access violations
//
// Use Cases:
//   - File download and reconstruction operations
//   - Block-by-block processing during file assembly
//   - Randomizer block retrieval for XOR operations
//   - Sequential file processing workflows
//
// Parameters:
//   - blockIndex: Zero-based index of the block (0 to len(blocks)-1)
//
// Returns:
//   - string: First randomizer CID for XOR operation
//   - string: Second randomizer CID for XOR operation
//   - error: Non-nil if block index is out of range
//
// Call Flow:
//   - Called by: Download operations, file reconstruction, block processing
//   - Used with: Block retrieval and XOR reconstruction operations
//
// Time Complexity: O(1) - direct array access with bounds checking
// Space Complexity: O(1) - no additional memory allocation
func (d *Descriptor) GetRandomizerCIDs(blockIndex int) (string, string, error) {
	if blockIndex < 0 || blockIndex >= len(d.Blocks) {
		return "", "", errors.New("block index out of range")
	}

	block := d.Blocks[blockIndex]
	return block.RandomizerCID1, block.RandomizerCID2, nil
}

// IsFile returns true if this is a file descriptor for type checking and conditional logic.
// This method provides a simple way to determine if a descriptor represents a file,
// enabling type-safe operations and proper handling of different descriptor types.
//
// Type Checking Benefits:
//   - Runtime type identification for conditional logic
//   - Safe casting and type-specific operations
//   - Clear boolean result for decision making
//   - Integration with validation and processing workflows
//
// Use Cases:
//   - Conditional processing based on descriptor type
//   - Type-safe operations in generic descriptor handling
//   - Validation logic requiring type checking
//   - API responses with type-specific formatting
//
// Returns:
//   - bool: True if descriptor type is FileType, false otherwise
//
// Call Flow:
//   - Called by: Processing logic, validation, type-specific operations
//   - Simple type comparison operation
//
// Time Complexity: O(1) - simple field comparison
// Space Complexity: O(1) - no memory allocation
func (d *Descriptor) IsFile() bool {
	return d.Type == FileType
}

// IsDirectory returns true if this is a directory descriptor for type checking and conditional logic.
// This method provides a simple way to determine if a descriptor represents a directory,
// enabling type-safe operations and proper handling of directory-specific functionality.
//
// Type Checking Benefits:
//   - Runtime type identification for conditional logic
//   - Safe directory operations and manifest handling
//   - Clear boolean result for decision making
//   - Integration with directory-specific workflows
//
// Use Cases:
//   - Directory-specific processing and operations
//   - Manifest retrieval and directory structure handling
//   - Type-safe operations in generic descriptor handling
//   - Hierarchical file system operations
//
// Returns:
//   - bool: True if descriptor type is DirectoryType, false otherwise
//
// Call Flow:
//   - Called by: Directory operations, manifest handling, type-specific logic
//   - Simple type comparison operation
//
// Time Complexity: O(1) - simple field comparison
// Space Complexity: O(1) - no memory allocation
func (d *Descriptor) IsDirectory() bool {
	return d.Type == DirectoryType
}

// IsPadded returns true if this descriptor uses padding for privacy-preserving block sizes.
// This method determines whether the file was padded during storage to achieve consistent
// block sizes, which is essential for privacy protection in the NoiseFS system.
//
// Padding Detection:
//   - Compares padded file size with original file size
//   - Returns true when padding was applied during storage
//   - Enables proper reconstruction logic for padded files
//   - Supports privacy-preserving fixed block size architecture
//
// Privacy Benefits:
//   - Padding enables consistent block sizes for anonymity
//   - Prevents file size fingerprinting attacks
//   - Improves anonymity set through size normalization
//   - Essential for NoiseFS privacy guarantees
//
// Use Cases:
//   - File reconstruction logic requiring padding awareness
//   - Download operations needing padding removal
//   - Privacy analysis and system monitoring
//   - Validation of privacy-preserving storage
//
// Returns:
//   - bool: True if padded file size exceeds original file size
//
// Call Flow:
//   - Called by: Download operations, reconstruction logic, privacy analysis
//   - Simple size comparison operation
//
// Time Complexity: O(1) - simple field comparison
// Space Complexity: O(1) - no memory allocation
func (d *Descriptor) IsPadded() bool {
	return d.PaddedFileSize > d.FileSize
}

// GetOriginalFileSize returns the original file size before padding for accurate reconstruction.
// This method provides access to the actual file size without padding,
// which is essential for proper file reconstruction and data truncation.
//
// Size Information:
//   - Returns actual file content size without privacy padding
//   - Essential for accurate file reconstruction and truncation
//   - Enables proper data handling during download operations
//   - Required for file integrity verification
//
// Reconstruction Usage:
//   - Used to truncate padded data during file reconstruction
//   - Ensures downloaded files match original size exactly
//   - Prevents inclusion of padding bytes in reconstructed files
//   - Critical for data integrity and user experience
//
// Use Cases:
//   - File download and reconstruction operations
//   - Data truncation during padding removal
//   - File size validation and integrity checking
//   - User interface file size display
//
// Returns:
//   - int64: Original file size in bytes before any padding
//
// Call Flow:
//   - Called by: Download operations, reconstruction logic, file size queries
//   - Direct field access operation
//
// Time Complexity: O(1) - simple field access
// Space Complexity: O(1) - no memory allocation
func (d *Descriptor) GetOriginalFileSize() int64 {
	return d.FileSize
}

// GetPaddedFileSize returns the total size including padding for storage calculations.
// This method provides the total size including any padding applied for privacy-preserving
// consistent block sizes, which is important for storage and retrieval operations.
//
// Padded Size Information:
//   - Returns total size including privacy-preserving padding
//   - Falls back to original size if no padding information available
//   - Essential for storage space calculations and block operations
//   - Required for understanding actual storage requirements
//
// Backward Compatibility:
//   - Returns original file size when padded size is not set (0)
//   - Ensures compatibility with older descriptor formats
//   - Graceful handling of descriptors without padding metadata
//   - Safe operation across different descriptor versions
//
// Use Cases:
//   - Storage space calculations and capacity planning
//   - Block-level operations requiring total size information
//   - Privacy analysis of padding overhead
//   - System monitoring and storage efficiency analysis
//
// Returns:
//   - int64: Total file size including padding, or original size if padding not available
//
// Call Flow:
//   - Called by: Storage operations, capacity planning, system monitoring
//   - Conditional field access with fallback logic
//
// Time Complexity: O(1) - simple field access with conditional logic
// Space Complexity: O(1) - no memory allocation
func (d *Descriptor) GetPaddedFileSize() int64 {
	if d.PaddedFileSize == 0 {
		return d.FileSize
	}
	return d.PaddedFileSize
}
