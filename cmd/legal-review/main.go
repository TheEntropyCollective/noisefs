package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/compliance"
)

func main() {
	var (
		outputDir   = flag.String("output", "./legal-review-output", "Output directory for review packages")
		format      = flag.String("format", "all", "Output format: json, html, markdown, or all")
		verbose     = flag.Bool("verbose", false, "Verbose output")
	)
	
	flag.Parse()
	
	// Create output directory
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}
	
	fmt.Println("NoiseFS Legal Review Package Generator")
	fmt.Println("=====================================")
	fmt.Printf("Output directory: %s\n", *outputDir)
	fmt.Printf("Format: %s\n", *format)
	fmt.Println()
	
	// Create generator
	generator := compliance.NewReviewGenerator()
	
	// Generate review package
	fmt.Println("Generating legal review package...")
	startTime := time.Now()
	
	pkg, err := generator.GenerateReviewPackage()
	if err != nil {
		log.Fatalf("Failed to generate review package: %v", err)
	}
	
	// Validate package
	if err := generator.ValidatePackage(pkg); err != nil {
		log.Fatalf("Package validation failed: %v", err)
	}
	
	fmt.Printf("✓ Package generated in %v\n", time.Since(startTime))
	fmt.Printf("✓ Package ID: %s\n", pkg.ID)
	
	// Export in requested formats
	if err := exportPackage(generator, pkg, *outputDir, *format); err != nil {
		log.Fatalf("Failed to export package: %v", err)
	}
	
	// Print summary
	printSummary(pkg, *verbose)
	
	fmt.Println("\n✅ Legal review package generation complete!")
	fmt.Printf("📁 Files saved to: %s\n", *outputDir)
}

func exportPackage(gen *compliance.ReviewGenerator, pkg *compliance.ReviewPackage, outputDir, format string) error {
	formats := []string{}
	
	switch format {
	case "all":
		formats = []string{"json", "html", "markdown"}
	case "json", "html", "markdown":
		formats = []string{format}
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
	
	for _, fmt := range formats {
		if err := exportFormat(gen, pkg, outputDir, fmt); err != nil {
			return err
		}
	}
	
	return nil
}

func exportFormat(gen *compliance.ReviewGenerator, pkg *compliance.ReviewPackage, outputDir, format string) error {
	filename := gen.GenerateFilename(pkg, format)
	filepath := filepath.Join(outputDir, filename)
	
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", filepath, err)
	}
	defer file.Close()
	
	fmt.Printf("Exporting %s format...", format)
	
	switch format {
	case "json":
		err = gen.ExportJSON(pkg, file)
	case "html":
		err = gen.ExportHTML(pkg, file)
	case "markdown":
		err = gen.ExportMarkdown(pkg, file)
	default:
		err = fmt.Errorf("unknown format: %s", format)
	}
	
	if err != nil {
		fmt.Printf(" ❌\n")
		return err
	}
	
	fmt.Printf(" ✓ (%s)\n", filename)
	return nil
}

func printSummary(pkg *compliance.ReviewPackage, verbose bool) {
	fmt.Println("\n📊 Review Package Summary")
	fmt.Println("========================")
	
	// Executive Summary highlights
	fmt.Println("\n🎯 Key Innovations:")
	for i, innovation := range pkg.ExecutiveSummary.KeyInnovations {
		if i >= 3 && !verbose {
			fmt.Printf("   ... and %d more\n", len(pkg.ExecutiveSummary.KeyInnovations)-3)
			break
		}
		fmt.Printf("   • %s\n", innovation)
	}
	
	// Test results summary
	if pkg.TestCaseAnalysis != nil && len(pkg.TestCaseAnalysis) > 0 {
		fmt.Printf("\n🧪 Test Cases Analyzed: %d\n", len(pkg.TestCaseAnalysis))
		
		successCount := 0
		for _, report := range pkg.TestCaseAnalysis {
			if report.SimulationResult != nil && 
			   report.SimulationResult.FinalOutcome != nil &&
			   report.SimulationResult.FinalOutcome.Success {
				successCount++
			}
		}
		
		successRate := float64(successCount) / float64(len(pkg.TestCaseAnalysis)) * 100
		fmt.Printf("   Success Rate: %.1f%%\n", successRate)
	}
	
	// Risk Assessment
	if pkg.RiskAssessment != nil {
		fmt.Println("\n⚠️  Risk Assessment:")
		
		// Find highest risk
		highestRisk := ""
		highestScore := 0.0
		
		for name, risk := range pkg.RiskAssessment.RiskMatrix {
			if risk.Score > highestScore {
				highestRisk = name
				highestScore = risk.Score
			}
		}
		
		if highestRisk != "" {
			fmt.Printf("   Highest Risk: %s (score: %.2f)\n", highestRisk, highestScore)
		}
		
		// Likely outcome
		if pkg.RiskAssessment.LikelyOutcome != nil {
			fmt.Printf("   Likely Outcome: %s\n", pkg.RiskAssessment.LikelyOutcome.Description)
		}
	}
	
	// Expert Questions
	if len(pkg.ExpertQuestions) > 0 {
		fmt.Printf("\n❓ Expert Questions: %d\n", len(pkg.ExpertQuestions))
		
		highPriority := 0
		for _, q := range pkg.ExpertQuestions {
			if q.Priority == "High" {
				highPriority++
			}
		}
		
		fmt.Printf("   High Priority: %d\n", highPriority)
		
		if verbose {
			fmt.Println("\n   Questions:")
			for _, q := range pkg.ExpertQuestions {
				if q.Priority == "High" {
					fmt.Printf("   • [%s] %s\n", q.Category, q.Question)
				}
			}
		}
	}
	
	// Compliance Status
	if pkg.ComplianceReport != nil {
		fmt.Println("\n✅ Compliance Status:")
		
		if pkg.ComplianceReport.DMCACompliance != nil {
			status := "❌ Non-compliant"
			if pkg.ComplianceReport.DMCACompliance.Compliant {
				status = "✅ Compliant"
			}
			fmt.Printf("   DMCA: %s\n", status)
		}
		
		if verbose && pkg.ComplianceReport.InternationalRegs != nil {
			fmt.Println("   International:")
			for region, status := range pkg.ComplianceReport.InternationalRegs {
				compliant := "❌"
				if status.Compliant {
					compliant = "✅"
				}
				fmt.Printf("     %s: %s\n", region, compliant)
			}
		}
	}
	
	// Recommendations
	if pkg.ExecutiveSummary != nil && len(pkg.ExecutiveSummary.Recommendations) > 0 {
		fmt.Println("\n💡 Top Recommendations:")
		for i, rec := range pkg.ExecutiveSummary.Recommendations {
			if i >= 3 && !verbose {
				fmt.Printf("   ... and %d more\n", len(pkg.ExecutiveSummary.Recommendations)-3)
				break
			}
			fmt.Printf("   %d. %s\n", i+1, rec)
		}
	}
}

// Additional helper for creating a summary report
func createSummaryReport(pkg *compliance.ReviewPackage, outputDir string) error {
	summaryPath := filepath.Join(outputDir, "SUMMARY.txt")
	
	file, err := os.Create(summaryPath)
	if err != nil {
		return err
	}
	defer file.Close()
	
	// Write summary header
	fmt.Fprintf(file, "NOISEFS LEGAL REVIEW SUMMARY\n")
	fmt.Fprintf(file, "===========================\n")
	fmt.Fprintf(file, "Generated: %s\n", pkg.GeneratedAt.Format("January 2, 2006 15:04:05"))
	fmt.Fprintf(file, "Package ID: %s\n\n", pkg.ID)
	
	// Write key findings
	fmt.Fprintf(file, "EXECUTIVE SUMMARY\n")
	fmt.Fprintf(file, "-----------------\n")
	fmt.Fprintf(file, "%s\n\n", pkg.ExecutiveSummary.Overview)
	
	fmt.Fprintf(file, "KEY LEGAL ADVANTAGES:\n")
	for _, adv := range pkg.ExecutiveSummary.LegalAdvantages {
		fmt.Fprintf(file, "• %s\n", adv)
	}
	
	fmt.Fprintf(file, "\nPRIMARY RISKS:\n")
	for _, risk := range pkg.ExecutiveSummary.PrimaryRisks {
		fmt.Fprintf(file, "• %s\n", risk)
	}
	
	fmt.Fprintf(file, "\nRECOMMENDATIONS:\n")
	for i, rec := range pkg.ExecutiveSummary.Recommendations {
		fmt.Fprintf(file, "%d. %s\n", i+1, rec)
	}
	
	fmt.Fprintf(file, "\nCONCLUSION:\n")
	fmt.Fprintf(file, "%s\n", pkg.ExecutiveSummary.Conclusion)
	
	return nil
}

// JSON pretty printer for verbose mode
func prettyPrintJSON(data interface{}) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Println(string(b))
}