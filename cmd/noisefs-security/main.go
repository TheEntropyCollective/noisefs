package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/TheEntropyCollective/noisefs/pkg/fuse"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/security"
	"golang.org/x/term"
)

func main() {
	var (
		command     = flag.String("command", "", "Command to execute: encrypt-index, migrate-index, secure-delete, memory-test")
		indexPath   = flag.String("index", "", "Path to index file (defaults to ~/.noisefs/index.json)")
		filePath    = flag.String("file", "", "Path to file for secure deletion")
		interactive = flag.Bool("interactive", true, "Use interactive password prompts")
	)
	flag.Parse()

	if *command == "" {
		fmt.Println("NoiseFS Security Tool")
		fmt.Println("Usage: noisefs-security -command=<cmd> [options]")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  encrypt-index    Encrypt an existing index file")
		fmt.Println("  migrate-index    Migrate unencrypted index to encrypted")
		fmt.Println("  secure-delete    Securely delete a file")
		fmt.Println("  memory-test      Test secure memory handling")
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  -index <path>    Index file path")
		fmt.Println("  -file <path>     File to securely delete")
		fmt.Println("  -interactive     Use interactive prompts (default: true)")
		os.Exit(1)
	}

	switch *command {
	case "encrypt-index":
		if err := encryptIndex(*indexPath, *interactive); err != nil {
			fmt.Fprintf(os.Stderr, "Error encrypting index: %v\n", err)
			os.Exit(1)
		}
	case "migrate-index":
		if err := migrateIndex(*indexPath, *interactive); err != nil {
			fmt.Fprintf(os.Stderr, "Error migrating index: %v\n", err)
			os.Exit(1)
		}
	case "secure-delete":
		if *filePath == "" {
			fmt.Fprintf(os.Stderr, "Error: -file parameter required for secure-delete\n")
			os.Exit(1)
		}
		if err := secureDeleteFile(*filePath); err != nil {
			fmt.Fprintf(os.Stderr, "Error securely deleting file: %v\n", err)
			os.Exit(1)
		}
	case "memory-test":
		memoryTest()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", *command)
		os.Exit(1)
	}
}

func encryptIndex(indexPath string, interactive bool) error {
	if indexPath == "" {
		var err error
		indexPath, err = fuse.GetDefaultIndexPath()
		if err != nil {
			return fmt.Errorf("failed to get default index path: %w", err)
		}
	}

	// Check if index file exists
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		return fmt.Errorf("index file does not exist: %s", indexPath)
	}

	// Get password
	var password string
	if interactive {
		fmt.Print("Enter password for encrypted index: ")
		passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		fmt.Println()
		password = string(passwordBytes)
	} else {
		fmt.Print("Password: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			password = strings.TrimSpace(scanner.Text())
		}
	}

	if password == "" {
		return fmt.Errorf("password cannot be empty")
	}

	// Migrate to encrypted format
	fmt.Printf("Migrating index to encrypted format...\n")
	if err := fuse.MigrateToEncrypted(indexPath, password); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	fmt.Printf("Index successfully encrypted: %s\n", indexPath)
	return nil
}

func migrateIndex(indexPath string, interactive bool) error {
	// Same as encrypt-index but with different messaging
	fmt.Println("Migrating unencrypted index to encrypted format...")
	return encryptIndex(indexPath, interactive)
}

func secureDeleteFile(filePath string) error {
	fmt.Printf("Securely deleting file: %s\n", filePath)
	
	config := security.SecurityConfig{
		AntiForensics: true,
		SecureMemory:  true,
	}
	
	securityManager := security.NewSecurityManager(config)
	defer securityManager.Shutdown()
	
	if err := securityManager.FileDeleter.SecureDelete(filePath); err != nil {
		return fmt.Errorf("secure deletion failed: %w", err)
	}
	
	fmt.Printf("File securely deleted: %s\n", filePath)
	return nil
}

func memoryTest() {
	fmt.Println("Testing secure memory handling...")
	
	config := security.SecurityConfig{
		SecureMemory:  true,
		AntiForensics: true,
	}
	
	mp := security.NewMemoryProtection(config.SecureMemory)
	
	// Create test data
	sensitiveData := []byte("This is sensitive data that should be securely cleared")
	fmt.Printf("Original data: %s\n", string(sensitiveData))
	
	// Create secure buffer
	buffer := security.NewSecureBuffer(len(sensitiveData), mp)
	copy(buffer.Data(), sensitiveData)
	
	fmt.Printf("Buffer data: %s\n", string(buffer.Data()))
	
	// Clear the buffer
	fmt.Println("Clearing buffer...")
	buffer.Clear()
	
	fmt.Printf("Buffer after clear: %v\n", buffer.Data())
	
	// Clear original data
	fmt.Println("Clearing original data...")
	mp.ClearSensitiveData(sensitiveData)
	
	fmt.Printf("Original data after clear: %v\n", sensitiveData)
	
	fmt.Println("Memory test completed successfully!")
}