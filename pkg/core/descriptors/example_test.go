package descriptors

import (
	"fmt"
	"time"
	
	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
)

// Example demonstrates the new directory functionality
func ExampleNewDirectoryDescriptor() {
	// Create a directory descriptor
	dirname := "my-documents"
	manifestCID := "QmExampleManifestCID123456"
	
	dirDesc := NewDirectoryDescriptor(dirname, manifestCID)
	
	fmt.Printf("Directory descriptor created:\n")
	fmt.Printf("  Type: %s\n", dirDesc.Type)
	fmt.Printf("  Version: %s\n", dirDesc.Version)
	fmt.Printf("  Filename: %s\n", dirDesc.Filename)
	fmt.Printf("  ManifestCID: %s\n", dirDesc.ManifestCID)
	fmt.Printf("  Is Directory: %t\n", dirDesc.IsDirectory())
	fmt.Printf("  Is File: %t\n", dirDesc.IsFile())
	
	// Output:
	// Directory descriptor created:
	//   Type: directory
	//   Version: 4.0
	//   Filename: my-documents
	//   ManifestCID: QmExampleManifestCID123456
	//   Is Directory: true
	//   Is File: false
}

// Example demonstrates directory manifest with encrypted filenames
func ExampleNewDirectoryManifest() {
	// Generate encryption keys
	masterKey, _ := crypto.GenerateKey("secure-password")
	dirKey, _ := crypto.DeriveDirectoryKey(masterKey, "/home/user/documents")
	
	// Create a directory manifest
	manifest := NewDirectoryManifest()
	
	// Add files with encrypted names
	files := []struct {
		name string
		cid  string
		size int64
	}{
		{"secret-document.pdf", "QmFile1CID", 1024},
		{"confidential-notes.txt", "QmFile2CID", 512},
		{"private-photos", "QmDir1CID", 0}, // subdirectory
	}
	
	for _, file := range files {
		// Encrypt the filename
		encryptedName, _ := crypto.EncryptFileName(file.name, dirKey)
		
		entryType := FileType
		if file.size == 0 && file.name == "private-photos" {
			entryType = DirectoryType
		}
		
		entry := DirectoryEntry{
			EncryptedName: encryptedName,
			CID:           file.cid,
			Type:          entryType,
			Size:          file.size,
			ModifiedAt:    time.Now(),
		}
		
		manifest.AddEntry(entry)
	}
	
	fmt.Printf("Directory manifest created:\n")
	fmt.Printf("  Entry count: %d\n", manifest.GetEntryCount())
	fmt.Printf("  Is empty: %t\n", manifest.IsEmpty())
	
	// Encrypt the entire manifest
	encryptedManifest, _ := EncryptManifest(manifest, dirKey)
	fmt.Printf("  Encrypted manifest size: %d+ bytes\n", len(encryptedManifest)/100*100)
	
	// Decrypt and verify
	decryptedManifest, _ := DecryptManifest(encryptedManifest, dirKey)
	fmt.Printf("  Decrypted entries: %d\n", decryptedManifest.GetEntryCount())
	
	// Decrypt the first filename to verify
	if len(decryptedManifest.Entries) > 0 {
		decryptedName, _ := crypto.DecryptFileName(decryptedManifest.Entries[0].EncryptedName, dirKey)
		fmt.Printf("  First file decrypted: %s\n", decryptedName)
	}
	
	// Output:
	// Directory manifest created:
	//   Entry count: 3
	//   Is empty: false
	//   Encrypted manifest size: 300+ bytes
	//   Decrypted entries: 3
	//   First file decrypted: secret-document.pdf
}

// Example demonstrates v4 file descriptor format
func ExampleDescriptor_backwardCompatibility() {
	// Create a v4 file descriptor (the current way)
	fileDesc := NewDescriptor("old-file.txt", 2048, 2048, 256)
	fileDesc.AddBlockTriple("QmData1", "QmRand1", "QmRand2")
	
	// This should work with v4 format
	fmt.Printf("Legacy file descriptor:\n")
	fmt.Printf("  Version: %s\n", fileDesc.Version)
	fmt.Printf("  Type: %s\n", fileDesc.Type)
	fmt.Printf("  Is File: %t\n", fileDesc.IsFile())
	fmt.Printf("  Is Directory: %t\n", fileDesc.IsDirectory())
	
	// JSON serialization should work
	jsonData, _ := fileDesc.ToJSON()
	fmt.Printf("  JSON serialization: %d+ bytes\n", len(jsonData)/100*100)
	
	// Deserialization should work
	loaded, _ := FromJSON(jsonData)
	fmt.Printf("  Loaded type: %s\n", loaded.Type)
	fmt.Printf("  Validation: %t\n", loaded.Validate() == nil)
	
	// Output:
	// Legacy file descriptor:
	//   Version: 4.0
	//   Type: file
	//   Is File: true
	//   Is Directory: false
	//   JSON serialization: 300+ bytes
	//   Loaded type: file
	//   Validation: true
}