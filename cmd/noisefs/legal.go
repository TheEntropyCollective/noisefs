package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// checkLegalDisclaimerAccepted checks if the legal disclaimer has been accepted recently
func checkLegalDisclaimerAccepted() bool {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	
	disclaimerFile := filepath.Join(homeDir, ".noisefs", "legal_accepted")
	
	info, err := os.Stat(disclaimerFile)
	if err != nil {
		return false
	}
	
	// Check if accepted within last 30 days
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	return info.ModTime().After(thirtyDaysAgo)
}

// showLegalDisclaimer displays the legal disclaimer and prompts for acceptance
func showLegalDisclaimer() {
	fmt.Println("================================================================================")
	fmt.Println("                            NOISEFS LEGAL NOTICE")
	fmt.Println("================================================================================")
	fmt.Println()
	fmt.Println("IMPORTANT: This software is designed for privacy-preserving distributed storage.")
	fmt.Println("By using NoiseFS, you acknowledge and agree to the following:")
	fmt.Println()
	fmt.Println("1. PRIVACY AND ANONYMITY:")
	fmt.Println("   • NoiseFS provides technical privacy through cryptographic anonymization")
	fmt.Println("   • The system is designed for plausible deniability and content protection")
	fmt.Println("   • No guarantees are made regarding legal immunity in all jurisdictions")
	fmt.Println()
	fmt.Println("2. LEGAL COMPLIANCE:")
	fmt.Println("   • Users are responsible for compliance with applicable laws")
	fmt.Println("   • Only store content you have legal rights to distribute")
	fmt.Println("   • Some jurisdictions may restrict anonymous file sharing")
	fmt.Println()
	fmt.Println("3. CONTENT RESPONSIBILITY:")
	fmt.Println("   • Do not store illegal content in any jurisdiction")
	fmt.Println("   • Do not store content that violates others' rights")
	fmt.Println("   • The software is provided 'AS IS' without warranties")
	fmt.Println()
	fmt.Println("4. TECHNICAL LIMITATIONS:")
	fmt.Println("   • Privacy depends on proper network deployment and usage")
	fmt.Println("   • Metadata leakage may occur through traffic analysis")
	fmt.Println("   • System security depends on cryptographic assumptions")
	fmt.Println()
	fmt.Println("For complete legal information, see:")
	fmt.Println("  • docs/legal_protections.md")
	fmt.Println("  • docs/privacy_protections.md")
	fmt.Println()
	fmt.Println("By continuing, you acknowledge you have read and understood these notices.")
	fmt.Println("================================================================================")
	fmt.Print("Do you accept these terms and understand the legal implications? (yes/no): ")
	
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	response := strings.ToLower(strings.TrimSpace(scanner.Text()))
	
	if response != "yes" && response != "y" {
		fmt.Println("\nLegal disclaimer not accepted. Exiting.")
		os.Exit(1)
	}
	
	// Record acceptance
	err := recordLegalAcceptance()
	if err != nil {
		fmt.Printf("Warning: Could not record legal acceptance: %v\n", err)
	}
	
	fmt.Println("\nLegal disclaimer accepted. Proceeding with NoiseFS operation.")
	fmt.Println("================================================================================")
	fmt.Println()
}

// recordLegalAcceptance creates a file to record that the legal disclaimer was accepted
func recordLegalAcceptance() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	
	noisefsDir := filepath.Join(homeDir, ".noisefs")
	err = os.MkdirAll(noisefsDir, 0755)
	if err != nil {
		return err
	}
	
	disclaimerFile := filepath.Join(noisefsDir, "legal_accepted")
	file, err := os.Create(disclaimerFile)
	if err != nil {
		return err
	}
	defer file.Close()
	
	_, err = file.WriteString(fmt.Sprintf("Legal disclaimer accepted at: %s\n", time.Now().Format(time.RFC3339)))
	return err
}