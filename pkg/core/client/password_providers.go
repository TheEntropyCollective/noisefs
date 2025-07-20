// Package noisefs provides common PasswordProvider implementations for client encryption configuration.
// This file contains ready-to-use password provider patterns that applications can use with
// the client's encryption configuration system, eliminating the need to implement custom
// password providers for common use cases.
//
// The providers support various password acquisition patterns including environment variables,
// static passwords (for development), interactive prompting, and custom callback functions.
// All providers implement the descriptors.PasswordProvider interface for compatibility.
package noisefs

import (
	"fmt"
	"os"

	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/util"
)

// StaticPasswordProvider creates a PasswordProvider that always returns the same password.
// This provider is useful for development environments, testing, or scenarios where the
// password is known at application startup and doesn't change during runtime.
//
// WARNING: Static passwords should be avoided in production environments as they may
// be exposed in memory dumps, configuration files, or application logs. Consider using
// environment variable or interactive providers for production deployments.
//
// Security Considerations:
//   - Password is stored in memory for the lifetime of the provider
//   - Suitable for development and testing scenarios only
//   - Should not be used with sensitive production data
//
// Parameters:
//   - password: The password to return on each call (should not be empty for security)
//
// Returns:
//   - descriptors.PasswordProvider: Provider function that returns the static password
//
// Example:
//   provider := noisefs.StaticPasswordProvider("my-dev-password")
//   client, err := noisefs.NewClientWithEncryption(storageManager, cache, provider)
//
// Time Complexity: O(1) - constant time password return
// Space Complexity: O(1) - stores single password string
func StaticPasswordProvider(password string) descriptors.PasswordProvider {
	return func() (string, error) {
		return password, nil
	}
}

// EnvironmentPasswordProvider creates a PasswordProvider that reads passwords from environment variables.
// This provider is suitable for production environments where passwords are managed through
// environment configuration, container orchestration, or deployment automation systems.
//
// The provider checks the specified environment variable on each call, allowing for
// dynamic password updates through environment changes without application restarts.
//
// Security Features:
//   - Passwords managed outside application code
//   - Compatible with container orchestration password management
//   - Supports dynamic password rotation through environment updates
//   - No password storage in application memory beyond call duration
//
// Parameters:
//   - envVar: Name of the environment variable containing the password
//
// Returns:
//   - descriptors.PasswordProvider: Provider function that reads from environment
//
// Example:
//   provider := noisefs.EnvironmentPasswordProvider("NOISEFS_ENCRYPTION_PASSWORD")
//   client, err := noisefs.NewClientWithEncryption(storageManager, cache, provider)
//
// Time Complexity: O(1) - environment variable lookup
// Space Complexity: O(1) - no persistent storage
func EnvironmentPasswordProvider(envVar string) descriptors.PasswordProvider {
	return func() (string, error) {
		password := os.Getenv(envVar)
		if password == "" {
			return "", fmt.Errorf("environment variable %s is not set or empty", envVar)
		}
		return password, nil
	}
}

// OptionalEnvironmentPasswordProvider creates a PasswordProvider that reads from environment variables with fallback.
// This provider attempts to read from the specified environment variable but returns an empty
// password if the variable is not set, allowing the client to fall back to unencrypted uploads
// when encryption is optional.
//
// This provider is useful for applications that support both encrypted and unencrypted modes
// depending on deployment configuration, providing encryption when available but not requiring it.
//
// Parameters:
//   - envVar: Name of the environment variable containing the password
//
// Returns:
//   - descriptors.PasswordProvider: Provider function that returns password or empty string
//
// Example:
//   provider := noisefs.OptionalEnvironmentPasswordProvider("NOISEFS_PASSWORD")
//   client, err := noisefs.NewClientWithEncryption(storageManager, cache, provider)
//
// Time Complexity: O(1) - environment variable lookup
// Space Complexity: O(1) - no persistent storage
func OptionalEnvironmentPasswordProvider(envVar string) descriptors.PasswordProvider {
	return func() (string, error) {
		return os.Getenv(envVar), nil // Returns empty string if not set
	}
}

// InteractivePasswordProvider creates a PasswordProvider that prompts for passwords interactively.
// This provider uses the terminal to securely prompt for passwords with hidden input,
// suitable for command-line applications and interactive tools.
//
// The provider validates terminal availability before prompting and supports custom
// prompt messages for user-friendly password collection.
//
// Interactive Features:
//   - Hidden password input (no echo to terminal)
//   - Terminal availability validation
//   - Custom prompt message support
//   - Secure password collection for CLI applications
//
// Parameters:
//   - prompt: Custom prompt message to display to the user
//
// Returns:
//   - descriptors.PasswordProvider: Provider function that prompts interactively
//
// Example:
//   provider := noisefs.InteractivePasswordProvider("Enter encryption password")
//   client, err := noisefs.NewClientWithEncryption(storageManager, cache, provider)
//
// Time Complexity: O(1) - limited by user input time
// Space Complexity: O(1) - temporary password storage only
func InteractivePasswordProvider(prompt string) descriptors.PasswordProvider {
	return func() (string, error) {
		return util.PromptPassword(prompt + ": ")
	}
}

// InteractivePasswordProviderWithConfirmation creates a PasswordProvider that prompts with confirmation.
// This provider extends the interactive password provider with confirmation prompting,
// ensuring password accuracy for critical operations like initial encryption setup.
//
// The provider prompts twice and validates that both password entries match before
// returning the password, reducing the risk of password entry errors.
//
// Parameters:
//   - prompt: Custom prompt message to display to the user
//
// Returns:
//   - descriptors.PasswordProvider: Provider function that prompts with confirmation
//
// Example:
//   provider := noisefs.InteractivePasswordProviderWithConfirmation("Set encryption password")
//   client, err := noisefs.NewClientWithMandatoryEncryption(storageManager, cache, provider)
//
// Time Complexity: O(1) - limited by user input time
// Space Complexity: O(1) - temporary password storage only
func InteractivePasswordProviderWithConfirmation(prompt string) descriptors.PasswordProvider {
	return func() (string, error) {
		return util.PromptPasswordWithConfirmation(prompt)
	}
}

// CallbackPasswordProvider creates a PasswordProvider from a custom callback function.
// This provider enables applications to implement custom password acquisition logic
// while maintaining compatibility with the client's encryption configuration system.
//
// The callback function receives no parameters and should return the password and any
// error encountered during password acquisition. This pattern supports integration
// with custom password management systems, secure vaults, or application-specific
// password collection mechanisms.
//
// Custom Integration Features:
//   - Integration with password managers and secure vaults
//   - Custom authentication workflows
//   - Application-specific password collection logic
//   - Dynamic password generation and rotation support
//
// Parameters:
//   - callback: Custom function that returns password and error
//
// Returns:
//   - descriptors.PasswordProvider: Provider function that delegates to callback
//
// Example:
//   provider := noisefs.CallbackPasswordProvider(func() (string, error) {
//       return myPasswordManager.GetPassword("noisefs-encryption")
//   })
//   client, err := noisefs.NewClientWithEncryption(storageManager, cache, provider)
//
// Time Complexity: O(f) where f is the complexity of the callback function
// Space Complexity: O(f) where f is the space complexity of the callback function
func CallbackPasswordProvider(callback func() (string, error)) descriptors.PasswordProvider {
	return callback
}

// ChainPasswordProvider creates a PasswordProvider that tries multiple providers in sequence.
// This provider attempts each provider in order until one succeeds or all fail,
// enabling fallback strategies and robust password acquisition workflows.
//
// The chain stops at the first provider that returns a non-empty password without error.
// If all providers fail or return empty passwords, the chain returns the last error encountered.
//
// Fallback Strategy Features:
//   - Multiple password acquisition strategies
//   - Robust fallback mechanisms for production environments
//   - Environment-to-interactive fallback patterns
//   - Graceful degradation of password acquisition methods
//
// Parameters:
//   - providers: Slice of PasswordProvider functions to try in order
//
// Returns:
//   - descriptors.PasswordProvider: Provider function that chains multiple providers
//
// Example:
//   provider := noisefs.ChainPasswordProvider([]descriptors.PasswordProvider{
//       noisefs.EnvironmentPasswordProvider("NOISEFS_PASSWORD"),
//       noisefs.InteractivePasswordProvider("Enter password"),
//   })
//   client, err := noisefs.NewClientWithEncryption(storageManager, cache, provider)
//
// Time Complexity: O(n*p) where n is the number of providers and p is the average provider complexity
// Space Complexity: O(1) - no persistent storage beyond individual provider requirements
func ChainPasswordProvider(providers []descriptors.PasswordProvider) descriptors.PasswordProvider {
	return func() (string, error) {
		var lastErr error
		for _, provider := range providers {
			if provider == nil {
				continue
			}
			password, err := provider()
			if err == nil && password != "" {
				return password, nil
			}
			if err != nil {
				lastErr = err
			}
		}
		if lastErr != nil {
			return "", fmt.Errorf("all password providers failed, last error: %w", lastErr)
		}
		return "", fmt.Errorf("all password providers returned empty passwords")
	}
}

// DefaultPasswordProvider creates a commonly-used password provider chain for production environments.
// This convenience function creates a sensible default provider chain that tries environment
// variables first and falls back to interactive prompting, suitable for most applications.
//
// Default Chain Strategy:
//   1. Try environment variable (NOISEFS_PASSWORD)
//   2. Fall back to interactive prompting if terminal is available
//   3. Fail if neither method works
//
// This provider implements a common production pattern where automated systems use
// environment variables while interactive systems prompt users directly.
//
// Returns:
//   - descriptors.PasswordProvider: Default provider chain for production use
//
// Example:
//   provider := noisefs.DefaultPasswordProvider()
//   client, err := noisefs.NewClientWithEncryption(storageManager, cache, provider)
//
// Time Complexity: O(1) - environment check plus potential user input
// Space Complexity: O(1) - no persistent storage
func DefaultPasswordProvider() descriptors.PasswordProvider {
	return ChainPasswordProvider([]descriptors.PasswordProvider{
		OptionalEnvironmentPasswordProvider("NOISEFS_PASSWORD"),
		InteractivePasswordProvider("Enter encryption password"),
	})
}