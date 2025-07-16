package compliance

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestReviewGeneratorCreation(t *testing.T) {
	gen := NewReviewGenerator()
	
	if gen == nil {
		t.Fatal("Failed to create review generator")
	}
	
	if gen.caseGenerator == nil {
		t.Error("Case generator not initialized")
	}
	
	if gen.courtSimulator == nil {
		t.Error("Court simulator not initialized")
	}
	
	if gen.precedentDB == nil {
		t.Error("Precedent database not initialized")
	}
	
	if len(gen.templates) == 0 {
		t.Error("Templates not initialized")
	}
}

func TestGenerateReviewPackage(t *testing.T) {
	gen := NewReviewGenerator()
	
	// Generate package
	pkg, err := gen.GenerateReviewPackage()
	if err != nil {
		t.Fatalf("Failed to generate review package: %v", err)
	}
	
	// Validate basic structure
	if pkg.ID == "" {
		t.Error("Package ID is empty")
	}
	
	if pkg.GeneratedAt.IsZero() {
		t.Error("Generation timestamp not set")
	}
	
	if pkg.Version == "" {
		t.Error("Version not set")
	}
	
	// Validate components
	if pkg.ExecutiveSummary == nil {
		t.Error("Executive summary missing")
	} else {
		validateExecutiveSummary(t, pkg.ExecutiveSummary)
	}
	
	if pkg.SystemArchitecture == nil {
		t.Error("System architecture missing")
	}
	
	if pkg.DefenseStrategy == nil {
		t.Error("Defense strategy missing")
	} else {
		validateDefenseStrategy(t, pkg.DefenseStrategy)
	}
	
	if len(pkg.TestCaseAnalysis) == 0 {
		t.Error("No test case analysis present")
	}
	
	if pkg.PrecedentAnalysis == nil {
		t.Error("Precedent analysis missing")
	}
	
	if pkg.RiskAssessment == nil {
		t.Error("Risk assessment missing")
	}
	
	if pkg.ComplianceReport == nil {
		t.Error("Compliance report missing")
	}
	
	if len(pkg.ExpertQuestions) == 0 {
		t.Error("No expert questions generated")
	}
	
	if len(pkg.SupportingDocs) == 0 {
		t.Error("No supporting documents")
	}
}

func validateExecutiveSummary(t *testing.T, summary *ExecutiveSummary) {
	if summary.Overview == "" {
		t.Error("Executive summary overview is empty")
	}
	
	if len(summary.KeyInnovations) == 0 {
		t.Error("No key innovations listed")
	}
	
	if len(summary.LegalAdvantages) == 0 {
		t.Error("No legal advantages listed")
	}
	
	if len(summary.PrimaryRisks) == 0 {
		t.Error("No primary risks identified")
	}
	
	if len(summary.Recommendations) == 0 {
		t.Error("No recommendations provided")
	}
	
	if summary.Conclusion == "" {
		t.Error("Executive summary conclusion is empty")
	}
}

func validateDefenseStrategy(t *testing.T, strategy *DefenseStrategy) {
	if len(strategy.PrimaryDefenses) == 0 {
		t.Error("No primary defenses listed")
	}
	
	// Check defense properties
	for i, defense := range strategy.PrimaryDefenses {
		if defense.Name == "" {
			t.Errorf("Defense %d has no name", i)
		}
		
		if defense.Description == "" {
			t.Errorf("Defense %d has no description", i)
		}
		
		if defense.Strength < 0 || defense.Strength > 1 {
			t.Errorf("Defense %d has invalid strength: %f", i, defense.Strength)
		}
		
		if len(defense.Precedents) == 0 {
			t.Errorf("Defense %d has no precedents", i)
		}
	}
	
	if len(strategy.PrecedentSupport) == 0 {
		t.Error("No precedent support mapping")
	}
	
	if len(strategy.ExpertWitnesses) == 0 {
		t.Error("No expert witnesses listed")
	}
	
	if strategy.DocumentationPlan == "" {
		t.Error("Documentation plan is empty")
	}
}

func TestExportFormats(t *testing.T) {
	gen := NewReviewGenerator()
	
	// Generate a package
	pkg, err := gen.GenerateReviewPackage()
	if err != nil {
		t.Fatalf("Failed to generate package: %v", err)
	}
	
	// Test JSON export
	t.Run("JSON Export", func(t *testing.T) {
		var buf bytes.Buffer
		err := gen.ExportJSON(pkg, &buf)
		if err != nil {
			t.Fatalf("Failed to export JSON: %v", err)
		}
		
		// Verify it's valid JSON
		var decoded ReviewPackage
		if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
			t.Fatalf("Invalid JSON output: %v", err)
		}
		
		// Verify key fields
		if decoded.ID != pkg.ID {
			t.Error("Package ID mismatch in JSON")
		}
	})
	
	// Test HTML export
	t.Run("HTML Export", func(t *testing.T) {
		var buf bytes.Buffer
		err := gen.ExportHTML(pkg, &buf)
		if err != nil {
			t.Fatalf("Failed to export HTML: %v", err)
		}
		
		html := buf.String()
		
		// Verify HTML structure
		if !strings.Contains(html, "<!DOCTYPE html>") {
			t.Error("Invalid HTML document")
		}
		
		if !strings.Contains(html, pkg.ID) {
			t.Error("Package ID not found in HTML")
		}
		
		if !strings.Contains(html, "Executive Summary") {
			t.Error("Executive Summary section missing")
		}
	})
	
	// Test Markdown export
	t.Run("Markdown Export", func(t *testing.T) {
		var buf bytes.Buffer
		err := gen.ExportMarkdown(pkg, &buf)
		if err != nil {
			t.Fatalf("Failed to export Markdown: %v", err)
		}
		
		markdown := buf.String()
		
		// Verify Markdown structure
		if !strings.Contains(markdown, "# NoiseFS Legal Review Package") {
			t.Error("Missing main header")
		}
		
		if !strings.Contains(markdown, pkg.ID) {
			t.Error("Package ID not found in Markdown")
		}
		
		if !strings.Contains(markdown, "## Executive Summary") {
			t.Error("Executive Summary section missing")
		}
	})
}

func TestValidatePackage(t *testing.T) {
	gen := NewReviewGenerator()
	
	// Test valid package
	validPkg := &ReviewPackage{
		ID:                 "test-123",
		GeneratedAt:        time.Now(),
		ExecutiveSummary:   &ExecutiveSummary{Overview: "Test"},
		SystemArchitecture: &ArchitectureOverview{},
		DefenseStrategy:    &DefenseStrategy{},
		TestCaseAnalysis:   []*TestCaseReport{{}},
	}
	
	err := gen.ValidatePackage(validPkg)
	if err != nil {
		t.Errorf("Valid package failed validation: %v", err)
	}
	
	// Test missing components
	testCases := []struct {
		name     string
		pkg      *ReviewPackage
		expected string
	}{
		{
			name: "Missing Executive Summary",
			pkg: &ReviewPackage{
				SystemArchitecture: &ArchitectureOverview{},
				DefenseStrategy:    &DefenseStrategy{},
				TestCaseAnalysis:   []*TestCaseReport{{}},
			},
			expected: "missing executive summary",
		},
		{
			name: "Missing System Architecture",
			pkg: &ReviewPackage{
				ExecutiveSummary: &ExecutiveSummary{},
				DefenseStrategy:  &DefenseStrategy{},
				TestCaseAnalysis: []*TestCaseReport{{}},
			},
			expected: "missing system architecture",
		},
		{
			name: "Missing Defense Strategy",
			pkg: &ReviewPackage{
				ExecutiveSummary:   &ExecutiveSummary{},
				SystemArchitecture: &ArchitectureOverview{},
				TestCaseAnalysis:   []*TestCaseReport{{}},
			},
			expected: "missing defense strategy",
		},
		{
			name: "No Test Cases",
			pkg: &ReviewPackage{
				ExecutiveSummary:   &ExecutiveSummary{},
				SystemArchitecture: &ArchitectureOverview{},
				DefenseStrategy:    &DefenseStrategy{},
				TestCaseAnalysis:   []*TestCaseReport{},
			},
			expected: "no test case analysis present",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := gen.ValidatePackage(tc.pkg)
			if err == nil {
				t.Error("Expected validation error")
			} else if err.Error() != tc.expected {
				t.Errorf("Expected error '%s', got '%s'", tc.expected, err.Error())
			}
		})
	}
}

func TestGenerateFilename(t *testing.T) {
	gen := NewReviewGenerator()
	
	pkg := &ReviewPackage{
		ID:          "test-123",
		GeneratedAt: time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC),
	}
	
	testCases := []struct {
		format   string
		expected string
	}{
		{"json", "noisefs-legal-review-20240115-143000.json"},
		{"html", "noisefs-legal-review-20240115-143000.html"},
		{"pdf", "noisefs-legal-review-20240115-143000.pdf"},
		{"md", "noisefs-legal-review-20240115-143000.md"},
	}
	
	for _, tc := range testCases {
		filename := gen.GenerateFilename(pkg, tc.format)
		if filename != tc.expected {
			t.Errorf("Format %s: expected %s, got %s", tc.format, tc.expected, filename)
		}
	}
}

func TestExpertQuestions(t *testing.T) {
	gen := NewReviewGenerator()
	questions := gen.generateExpertQuestions()
	
	if len(questions) == 0 {
		t.Fatal("No expert questions generated")
	}
	
	// Verify question structure
	highPriorityCount := 0
	categoriesFound := make(map[string]bool)
	
	for i, q := range questions {
		if q.ID == "" {
			t.Errorf("Question %d has no ID", i)
		}
		
		if q.Category == "" {
			t.Errorf("Question %d has no category", i)
		} else {
			categoriesFound[q.Category] = true
		}
		
		if q.Question == "" {
			t.Errorf("Question %d has no question text", i)
		}
		
		if q.Context == "" {
			t.Errorf("Question %d has no context", i)
		}
		
		if q.Priority == "High" {
			highPriorityCount++
		}
		
		if len(q.RelatedCases) == 0 {
			t.Errorf("Question %d has no related cases", i)
		}
	}
	
	// Verify we have diverse categories
	expectedCategories := []string{"Architecture", "DMCA Compliance", "Liability"}
	for _, cat := range expectedCategories {
		if !categoriesFound[cat] {
			t.Errorf("Missing expected category: %s", cat)
		}
	}
	
	// Verify we have high priority questions
	if highPriorityCount == 0 {
		t.Error("No high priority questions found")
	}
}

func TestRiskAssessment(t *testing.T) {
	gen := NewReviewGenerator()
	risk := gen.assessRisks()
	
	if risk == nil {
		t.Fatal("Risk assessment is nil")
	}
	
	// Check risk matrix
	if len(risk.RiskMatrix) == 0 {
		t.Error("Risk matrix is empty")
	}
	
	for name, assessment := range risk.RiskMatrix {
		if assessment.Description == "" {
			t.Errorf("Risk %s has no description", name)
		}
		
		if assessment.Likelihood < 0 || assessment.Likelihood > 1 {
			t.Errorf("Risk %s has invalid likelihood: %f", name, assessment.Likelihood)
		}
		
		if assessment.Impact < 0 || assessment.Impact > 1 {
			t.Errorf("Risk %s has invalid impact: %f", name, assessment.Impact)
		}
		
		expectedScore := assessment.Likelihood * assessment.Impact
		if math.Abs(assessment.Score-expectedScore) > 1e-9 {
			t.Errorf("Risk %s has incorrect score: expected %f, got %f", 
				name, expectedScore, assessment.Score)
		}
	}
	
	// Check mitigation plans
	if len(risk.MitigationPlan) == 0 {
		t.Error("No mitigation plans")
	}
	
	// Check scenarios
	if risk.WorstCase == nil {
		t.Error("Worst case scenario missing")
	}
	
	if risk.BestCase == nil {
		t.Error("Best case scenario missing")
	}
	
	if risk.LikelyOutcome == nil {
		t.Error("Likely outcome missing")
	}
	
	// Validate scenario outcomes sum to 1.0
	scenarios := []*ScenarioAnalysis{risk.WorstCase, risk.BestCase, risk.LikelyOutcome}
	for i, scenario := range scenarios {
		sum := 0.0
		for _, prob := range scenario.Outcomes {
			sum += prob
		}
		
		if sum < 0.99 || sum > 1.01 { // Allow small floating point error
			t.Errorf("Scenario %d outcomes don't sum to 1.0: %f", i, sum)
		}
	}
}

func TestComplianceReport(t *testing.T) {
	gen := NewReviewGenerator()
	compliance := gen.generateComplianceReport()
	
	if compliance == nil {
		t.Fatal("Compliance report is nil")
	}
	
	// Check DMCA compliance
	if compliance.DMCACompliance == nil {
		t.Error("DMCA compliance status missing")
	} else {
		if !compliance.DMCACompliance.Compliant {
			t.Error("DMCA should be compliant")
		}
		
		if len(compliance.DMCACompliance.Requirements) == 0 {
			t.Error("No DMCA requirements listed")
		}
	}
	
	// Check international regulations
	if len(compliance.InternationalRegs) == 0 {
		t.Error("No international regulations")
	}
	
	// Check data protection
	if compliance.DataProtection == nil {
		t.Error("Data protection status missing")
	}
	
	// Check industry standards
	if len(compliance.IndustryStandards) == 0 {
		t.Error("No industry standards listed")
	}
}

// Benchmark tests
func BenchmarkGenerateReviewPackage(b *testing.B) {
	gen := NewReviewGenerator()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := gen.GenerateReviewPackage()
		if err != nil {
			b.Fatalf("Failed to generate package: %v", err)
		}
	}
}

func BenchmarkExportJSON(b *testing.B) {
	gen := NewReviewGenerator()
	pkg, _ := gen.GenerateReviewPackage()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		err := gen.ExportJSON(pkg, &buf)
		if err != nil {
			b.Fatalf("Failed to export JSON: %v", err)
		}
	}
}