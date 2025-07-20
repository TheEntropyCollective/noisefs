package util

import (
	"testing"
)

func TestPromptPassword_NonInteractiveTerminal(t *testing.T) {
	// Test with non-interactive environment
	_, err := PromptPassword("Enter password: ")
	if err == nil {
		t.Error("Expected error for non-interactive terminal")
	}
	if err.Error() != "interactive password prompting requires a terminal" {
		t.Errorf("Expected specific error message, got: %s", err.Error())
	}
}

func TestPromptPasswordWithConfirmation_NonInteractiveTerminal(t *testing.T) {
	// Test with non-interactive environment
	_, err := PromptPasswordWithConfirmation("Enter password")
	if err == nil {
		t.Error("Expected error for non-interactive terminal")
	}
	if err.Error() != "interactive password prompting requires a terminal" {
		t.Errorf("Expected specific error message, got: %s", err.Error())
	}
}

func TestPromptYesNo_NonInteractiveTerminal(t *testing.T) {
	// Test with non-interactive environment
	_, err := PromptYesNo("Continue?")
	if err == nil {
		t.Error("Expected error for non-interactive terminal")
	}
	if err.Error() != "interactive prompting requires a terminal" {
		t.Errorf("Expected specific error message, got: %s", err.Error())
	}
}

// Note: Interactive tests would require a real terminal and user input,
// so we focus on testing the error conditions here.
// In a real environment, these functions would work correctly with actual terminals.