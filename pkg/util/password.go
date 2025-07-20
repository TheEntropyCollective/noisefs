package util

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"
)

// PromptPassword prompts the user for a password with hidden input
func PromptPassword(prompt string) (string, error) {
	// Check if we're running in an interactive terminal
	if !term.IsTerminal(int(syscall.Stdin)) {
		return "", fmt.Errorf("interactive password prompting requires a terminal")
	}

	fmt.Fprint(os.Stderr, prompt)
	password, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Fprintln(os.Stderr) // New line after hidden input
	
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}

	return string(password), nil
}

// PromptPasswordWithConfirmation prompts for a password and asks for confirmation
func PromptPasswordWithConfirmation(prompt string) (string, error) {
	password, err := PromptPassword(prompt + ": ")
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(password) == "" {
		return "", fmt.Errorf("password cannot be empty")
	}

	confirm, err := PromptPassword("Confirm password: ")
	if err != nil {
		return "", err
	}

	if password != confirm {
		return "", fmt.Errorf("passwords do not match")
	}

	return password, nil
}

// PromptYesNo prompts the user for a yes/no response
func PromptYesNo(prompt string) (bool, error) {
	if !term.IsTerminal(int(syscall.Stdin)) {
		return false, fmt.Errorf("interactive prompting requires a terminal")
	}

	fmt.Fprint(os.Stderr, prompt+" (y/n): ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes", nil
}