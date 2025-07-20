package security

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// SensitivePatterns defines regex patterns for detecting sensitive information
type SensitivePatterns struct {
	// Unix/Linux paths (e.g., /home/user/path, /usr/local/bin)
	UnixPaths *regexp.Regexp
	// Windows paths (e.g., C:\Users\path, D:\Windows\System32)
	WindowsPaths *regexp.Regexp
	// IP addresses (e.g., 192.168.1.1, 10.0.0.1)
	IPAddresses *regexp.Regexp
	// MAC addresses (e.g., 00:11:22:33:44:55)
	MACAddresses *regexp.Regexp
	// Email addresses (e.g., user@domain.com)
	EmailAddresses *regexp.Regexp
	// Hostnames with TLDs (e.g., server.example.com)
	Hostnames *regexp.Regexp
}

var (
	// Global compiled patterns for performance
	globalPatterns *SensitivePatterns
	patternOnce    sync.Once
)

// initializePatterns compiles and caches regex patterns for performance
func initializePatterns() *SensitivePatterns {
	return &SensitivePatterns{
		// Unix paths: absolute paths starting with /
		// Matches /src/file.txt, /home/user/file, /var/log/, but not ./relative
		// Use word boundaries and allow for various path structures
		UnixPaths: regexp.MustCompile(`\b/[a-zA-Z0-9._-]+(?:/[a-zA-Z0-9._-]+)*/?(?:\.[a-zA-Z0-9]+)?\b`),
		// Windows paths: C:\path\to\file or \\server\share\path
		WindowsPaths: regexp.MustCompile(`(?:[A-Z]:\\|\\\\[^\\]+\\)[^\\/:*?"<>|\r\n]+(?:\\[^\\/:*?"<>|\r\n]+)*\\?`),
		// IPv4 addresses: only match full IP addresses with word boundaries
		IPAddresses: regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`),
		// MAC addresses
		MACAddresses: regexp.MustCompile(`\b(?:[0-9a-fA-F]{2}[:-]){5}[0-9a-fA-F]{2}\b`),
		// Email addresses: full email pattern with @ and domain
		EmailAddresses: regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`),
		// Hostnames: match domain names but exclude file extensions
		// Must have at least one subdomain (server.domain.com) and not end with common file extensions
		Hostnames: regexp.MustCompile(`\b[a-zA-Z0-9][a-zA-Z0-9-]{1,}\.[a-zA-Z][a-zA-Z0-9-]{1,}\.[a-zA-Z]{2,}\b`),
	}
}

// getPatterns returns cached compiled patterns
func getPatterns() *SensitivePatterns {
	patternOnce.Do(func() {
		globalPatterns = initializePatterns()
	})
	return globalPatterns
}

// SanitizeError sanitizes error messages by removing sensitive information
// while preserving useful debugging context.
//
// Parameters:
//   - err: The original error to sanitize
//   - publicPath: Optional public path that's safe to display (e.g., relative to user's working directory)
//   - preserveContext: If true, maintains more context while still removing sensitive details
//
// Returns a new error with sanitized message
func SanitizeError(err error, publicPath string, preserveContext bool) error {
	if err == nil {
		return nil
	}

	message := err.Error()
	patterns := getPatterns()

	// Get current working directory for safe context preservation
	cwd, _ := os.Getwd()

	// Replace sensitive patterns with generic placeholders
	message = sanitizeMessage(message, patterns, publicPath, cwd, preserveContext)

	return fmt.Errorf("%s", message)
}

// SanitizeString sanitizes a string message using the same logic as SanitizeError
func SanitizeString(message, publicPath string, preserveContext bool) string {
	if message == "" {
		return message
	}

	patterns := getPatterns()
	cwd, _ := os.Getwd()

	return sanitizeMessage(message, patterns, publicPath, cwd, preserveContext)
}

// sanitizeMessage performs the actual message sanitization
func sanitizeMessage(message string, patterns *SensitivePatterns, publicPath, cwd string, preserveContext bool) string {
	// Replace Unix paths - handle overlapping matches by processing longest first
	message = patterns.UnixPaths.ReplaceAllStringFunc(message, func(match string) string {
		return sanitizePath(match, publicPath, cwd, preserveContext)
	})

	// Replace Windows paths
	message = patterns.WindowsPaths.ReplaceAllStringFunc(message, func(match string) string {
		return sanitizePath(match, publicPath, cwd, preserveContext)
	})

	// Replace IP addresses
	message = patterns.IPAddresses.ReplaceAllStringFunc(message, func(match string) string {
		if preserveContext {
			// Keep private IP ranges for debugging
			if strings.HasPrefix(match, "127.") || strings.HasPrefix(match, "192.168.") || strings.HasPrefix(match, "10.") {
				return "[LOCAL_IP]"
			}
		}
		return "[IP_ADDRESS]"
	})

	// Replace MAC addresses
	message = patterns.MACAddresses.ReplaceAllString(message, "[MAC_ADDRESS]")

	// Replace email addresses
	message = patterns.EmailAddresses.ReplaceAllString(message, "[EMAIL]")

	// Replace hostnames/domains - be very conservative
	message = patterns.Hostnames.ReplaceAllStringFunc(message, func(match string) string {
		// Don't replace common service domains that aren't sensitive
		commonDomains := []string{"localhost", "example.com", "test.com"}
		for _, domain := range commonDomains {
			if strings.Contains(match, domain) {
				return match
			}
		}
		// Don't replace if it looks like a filename with extension
		fileExtensions := []string{".txt", ".pdf", ".doc", ".jpg", ".png", ".log", ".json", ".xml", ".html"}
		for _, ext := range fileExtensions {
			if strings.HasSuffix(strings.ToLower(match), ext) {
				return match
			}
		}
		return "[HOSTNAME]"
	})

	return message
}

// sanitizePath handles path sanitization with context preservation options
func sanitizePath(path, publicPath, cwd string, preserveContext bool) string {
	// If this is the public path or matches exactly, allow it through
	if publicPath != "" {
		if path == publicPath || strings.HasSuffix(path, publicPath) {
			return publicPath
		}
	}

	// If preserving context and path is relative to current working directory, show relative portion
	if preserveContext && cwd != "" {
		if strings.HasPrefix(path, cwd) {
			relPath, err := filepath.Rel(cwd, path)
			if err == nil && !strings.HasPrefix(relPath, "..") {
				return "./" + relPath
			}
		}
	}

	// For absolute paths with just filename, check if it's a directory
	if preserveContext {
		if strings.HasSuffix(path, "/") {
			return "[DIRECTORY]"
		}
	}

	// Determine if it's a file or directory and preserve that context
	if preserveContext {
		if strings.HasSuffix(path, "/") || strings.HasSuffix(path, "\\") {
			return "[DIRECTORY]"
		}
		
		// Check if it looks like a file with extension
		base := filepath.Base(path)
		if strings.Contains(base, ".") {
			ext := filepath.Ext(base)
			if ext != "" {
				return "[FILE" + ext + "]"
			}
		}
		
		return "[PATH]"
	}

	return "[PATH]"
}

// SanitizeErrorForUser sanitizes errors for user-facing display
// This is a convenience function that uses conservative settings
func SanitizeErrorForUser(err error, userPath string) error {
	return SanitizeError(err, userPath, false)
}

// SanitizeErrorForDebug sanitizes errors for debug output
// This preserves more context while still removing sensitive details
func SanitizeErrorForDebug(err error, userPath string) error {
	return SanitizeError(err, userPath, true)
}

// SanitizeForLogging sanitizes messages for internal logging
// This removes all sensitive information for safe log storage
func SanitizeForLogging(message string) string {
	return SanitizeString(message, "", false)
}

// SanitizeForVerbose sanitizes messages for verbose/debug output
// This preserves useful debugging context while removing sensitive details
func SanitizeForVerbose(message, userContext string) string {
	return SanitizeString(message, userContext, true)
}