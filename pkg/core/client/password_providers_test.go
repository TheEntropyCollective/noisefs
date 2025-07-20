package noisefs

import (
	"errors"
	"os"
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
)

func TestStaticPasswordProvider(t *testing.T) {
	expectedPassword := "test-static-password"
	provider := StaticPasswordProvider(expectedPassword)

	password, err := provider()
	if err != nil {
		t.Fatalf("StaticPasswordProvider returned error: %v", err)
	}

	if password != expectedPassword {
		t.Errorf("Expected password %q, got %q", expectedPassword, password)
	}

	// Test that it returns the same password on multiple calls
	password2, err2 := provider()
	if err2 != nil {
		t.Fatalf("StaticPasswordProvider returned error on second call: %v", err2)
	}

	if password2 != expectedPassword {
		t.Errorf("Expected same password on second call, got %q", password2)
	}
}

func TestEnvironmentPasswordProvider(t *testing.T) {
	envVar := "TEST_NOISEFS_PASSWORD"
	expectedPassword := "test-env-password"

	// Test with environment variable set
	os.Setenv(envVar, expectedPassword)
	defer os.Unsetenv(envVar)

	provider := EnvironmentPasswordProvider(envVar)
	password, err := provider()
	if err != nil {
		t.Fatalf("EnvironmentPasswordProvider returned error: %v", err)
	}

	if password != expectedPassword {
		t.Errorf("Expected password %q, got %q", expectedPassword, password)
	}

	// Test with environment variable unset
	os.Unsetenv(envVar)
	provider2 := EnvironmentPasswordProvider(envVar)
	_, err2 := provider2()
	if err2 == nil {
		t.Error("Expected error when environment variable is not set")
	}

	// Test with empty environment variable
	os.Setenv(envVar, "")
	provider3 := EnvironmentPasswordProvider(envVar)
	_, err3 := provider3()
	if err3 == nil {
		t.Error("Expected error when environment variable is empty")
	}
}

func TestOptionalEnvironmentPasswordProvider(t *testing.T) {
	envVar := "TEST_NOISEFS_OPTIONAL_PASSWORD"
	expectedPassword := "test-optional-password"

	// Test with environment variable set
	os.Setenv(envVar, expectedPassword)
	defer os.Unsetenv(envVar)

	provider := OptionalEnvironmentPasswordProvider(envVar)
	password, err := provider()
	if err != nil {
		t.Fatalf("OptionalEnvironmentPasswordProvider returned error: %v", err)
	}

	if password != expectedPassword {
		t.Errorf("Expected password %q, got %q", expectedPassword, password)
	}

	// Test with environment variable unset - should return empty string without error
	os.Unsetenv(envVar)
	provider2 := OptionalEnvironmentPasswordProvider(envVar)
	password2, err2 := provider2()
	if err2 != nil {
		t.Fatalf("OptionalEnvironmentPasswordProvider returned error when env var unset: %v", err2)
	}

	if password2 != "" {
		t.Errorf("Expected empty password when env var unset, got %q", password2)
	}

	// Test with empty environment variable - should return empty string without error
	os.Setenv(envVar, "")
	provider3 := OptionalEnvironmentPasswordProvider(envVar)
	password3, err3 := provider3()
	if err3 != nil {
		t.Fatalf("OptionalEnvironmentPasswordProvider returned error when env var empty: %v", err3)
	}

	if password3 != "" {
		t.Errorf("Expected empty password when env var empty, got %q", password3)
	}
}

func TestCallbackPasswordProvider(t *testing.T) {
	expectedPassword := "test-callback-password"

	// Test successful callback
	callback := func() (string, error) {
		return expectedPassword, nil
	}

	provider := CallbackPasswordProvider(callback)
	password, err := provider()
	if err != nil {
		t.Fatalf("CallbackPasswordProvider returned error: %v", err)
	}

	if password != expectedPassword {
		t.Errorf("Expected password %q, got %q", expectedPassword, password)
	}

	// Test callback with error
	expectedError := errors.New("test callback error")
	errorCallback := func() (string, error) {
		return "", expectedError
	}

	provider2 := CallbackPasswordProvider(errorCallback)
	_, err2 := provider2()
	if err2 == nil {
		t.Error("Expected error from callback, got nil")
	}

	if err2 != expectedError {
		t.Errorf("Expected specific error %v, got %v", expectedError, err2)
	}
}

func TestChainPasswordProvider(t *testing.T) {
	// Test successful chain - first provider succeeds
	password1 := "first-password"
	provider1 := func() (string, error) { return password1, nil }
	provider2 := func() (string, error) { return "second-password", nil }

	chain := ChainPasswordProvider([]descriptors.PasswordProvider{provider1, provider2})
	password, err := chain()
	if err != nil {
		t.Fatalf("ChainPasswordProvider returned error: %v", err)
	}

	if password != password1 {
		t.Errorf("Expected first password %q, got %q", password1, password)
	}

	// Test fallback - first provider fails, second succeeds
	password2 := "second-password"
	providerFail := func() (string, error) { return "", errors.New("first provider failed") }
	providerSuccess := func() (string, error) { return password2, nil }

	chain2 := ChainPasswordProvider([]descriptors.PasswordProvider{providerFail, providerSuccess})
	password, err = chain2()
	if err != nil {
		t.Fatalf("ChainPasswordProvider returned error: %v", err)
	}

	if password != password2 {
		t.Errorf("Expected second password %q, got %q", password2, password)
	}

	// Test all providers fail
	providerFail1 := func() (string, error) { return "", errors.New("first failed") }
	providerFail2 := func() (string, error) { return "", errors.New("second failed") }

	chain3 := ChainPasswordProvider([]descriptors.PasswordProvider{providerFail1, providerFail2})
	_, err = chain3()
	if err == nil {
		t.Error("Expected error when all providers fail")
	}

	// Test provider returns empty password
	providerEmpty := func() (string, error) { return "", nil }
	providerSuccess2 := func() (string, error) { return "success", nil }

	chain4 := ChainPasswordProvider([]descriptors.PasswordProvider{providerEmpty, providerSuccess2})
	password, err = chain4()
	if err != nil {
		t.Fatalf("ChainPasswordProvider returned error: %v", err)
	}

	if password != "success" {
		t.Errorf("Expected success password, got %q", password)
	}

	// Test nil provider in chain
	chain5 := ChainPasswordProvider([]descriptors.PasswordProvider{nil, providerSuccess2})
	password, err = chain5()
	if err != nil {
		t.Fatalf("ChainPasswordProvider returned error with nil provider: %v", err)
	}

	if password != "success" {
		t.Errorf("Expected success password with nil provider, got %q", password)
	}
}

func TestDefaultPasswordProvider(t *testing.T) {
	// Test default provider creation
	provider := DefaultPasswordProvider()
	if provider == nil {
		t.Fatal("DefaultPasswordProvider returned nil")
	}

	// The default provider should work but will try environment first
	// We can't easily test the interactive part in unit tests
	envVar := "NOISEFS_PASSWORD"
	expectedPassword := "test-default-password"

	// Set environment variable to test the environment path
	os.Setenv(envVar, expectedPassword)
	defer os.Unsetenv(envVar)

	password, err := provider()
	if err != nil {
		t.Fatalf("DefaultPasswordProvider returned error: %v", err)
	}

	if password != expectedPassword {
		t.Errorf("Expected password %q, got %q", expectedPassword, password)
	}
}