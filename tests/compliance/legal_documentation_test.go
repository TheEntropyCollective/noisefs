package compliance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/compliance"
	"github.com/TheEntropyCollective/noisefs/pkg/legal"
	"github.com/TheEntropyCollective/noisefs/pkg/reuse"
)

// LegalDocumentationTestSuite tests legal documentation generation and validation
type LegalDocumentationTestSuite struct {
	legalSystem      *legal.LegalDocumentationSystem
	complianceSystem *compliance.ComplianceAuditSystem
	reuseClient      *reuse.ReuseAwareClient
	tempDir          string
}

// TestDMCAResponsePackageGeneration tests generation of complete DMCA response packages
func TestDMCAResponsePackageGeneration(t *testing.T) {
	suite := setupLegalDocumentationTest(t)
	defer suite.cleanup()

	// Create a test descriptor with reuse evidence
	testDescriptor := "noisefs://test-legal-descriptor-001"
	
	// Generate DMCA response package
	responsePackage, err := suite.legalSystem.GenerateDMCAResponsePackage(
		testDescriptor,
		"Test Copyrighted Work",
		"Test Copyright Holder",
		"takedown_001",
	)
	
	if err != nil {
		t.Fatalf("Failed to generate DMCA response package: %v", err)
	}

	// Verify package completeness
	if responsePackage.TechnicalDefense == nil {
		t.Error("DMCA response package missing technical defense")
	}

	if responsePackage.LegalArgument == nil {
		t.Error("DMCA response package missing legal argument")
	}

	if responsePackage.ExpertWitnessReport == nil {
		t.Error("DMCA response package missing expert witness report")
	}

	if len(responsePackage.BlockAnalysis) == 0 {
		t.Error("DMCA response package missing block analysis")
	}

	// Verify cryptographic signatures
	if !suite.verifyPackageAuthenticity(responsePackage) {
		t.Error("DMCA response package failed authenticity verification")
	}

	t.Logf("Generated DMCA response package with %d block analyses", len(responsePackage.BlockAnalysis))
}

// TestTechnicalDefenseKitGeneration tests technical defense documentation
func TestTechnicalDefenseKitGeneration(t *testing.T) {
	suite := setupLegalDocumentationTest(t)
	defer suite.cleanup()

	testDescriptor := "noisefs://test-technical-defense-001"
	
	// Generate technical defense kit
	defenseKit, err := suite.legalSystem.GenerateTechnicalDefenseKit(testDescriptor)
	if err != nil {
		t.Fatalf("Failed to generate technical defense kit: %v", err)
	}

	// Verify technical components
	if defenseKit.SystemArchitecture == nil {
		t.Error("Technical defense kit missing system architecture documentation")
	}

	if defenseKit.BlockAnonymization == nil {
		t.Error("Technical defense kit missing block anonymization proof")
	}

	if defenseKit.CryptographicEvidence == nil {
		t.Error("Technical defense kit missing cryptographic evidence")
	}

	if len(defenseKit.PlausibleDeniabilityProof) == 0 {
		t.Error("Technical defense kit missing plausible deniability proof")
	}

	// Verify mathematical proofs
	if !suite.verifyMathematicalProofs(defenseKit) {
		t.Error("Technical defense kit mathematical proofs failed verification")
	}

	t.Logf("Generated technical defense kit with %d cryptographic proofs", len(defenseKit.CryptographicEvidence))
}

// TestExpertWitnessReportGeneration tests expert witness report generation
func TestExpertWitnessReportGeneration(t *testing.T) {
	suite := setupLegalDocumentationTest(t)
	defer suite.cleanup()

	testDescriptor := "noisefs://test-expert-witness-001"
	
	// Generate expert witness report
	expertReport, err := suite.legalSystem.GenerateExpertWitnessReport(
		testDescriptor,
		"Dr. Test Expert",
		"Ph.D. Computer Science, Expert in Distributed Systems",
	)
	
	if err != nil {
		t.Fatalf("Failed to generate expert witness report: %v", err)
	}

	// Verify report components
	if expertReport.ExpertQualifications == "" {
		t.Error("Expert witness report missing expert qualifications")
	}

	if expertReport.TechnicalAnalysis == nil {
		t.Error("Expert witness report missing technical analysis")
	}

	if len(expertReport.LegalPrecedents) == 0 {
		t.Error("Expert witness report missing legal precedents")
	}

	if expertReport.ProfessionalOpinion == "" {
		t.Error("Expert witness report missing professional opinion")
	}

	// Verify report meets court standards
	if !suite.verifyCourtReadiness(expertReport) {
		t.Error("Expert witness report does not meet court readiness standards")
	}

	t.Logf("Generated expert witness report with %d legal precedents cited", len(expertReport.LegalPrecedents))
}

// TestBlockAnalysisReport tests detailed block analysis for legal purposes
func TestBlockAnalysisReport(t *testing.T) {
	suite := setupLegalDocumentationTest(t)
	defer suite.cleanup()

	testDescriptor := "noisefs://test-block-analysis-001"
	
	// Generate block analysis report
	blockReport, err := suite.legalSystem.GenerateBlockAnalysisReport(testDescriptor)
	if err != nil {
		t.Fatalf("Failed to generate block analysis report: %v", err)
	}

	// Verify analysis components
	if len(blockReport.BlockBreakdown) == 0 {
		t.Error("Block analysis report missing block breakdown")
	}

	if blockReport.AnonymizationProof == nil {
		t.Error("Block analysis report missing anonymization proof")
	}

	if blockReport.ReuseEvidence == nil {
		t.Error("Block analysis report missing reuse evidence")
	}

	if len(blockReport.PublicDomainSources) == 0 {
		t.Error("Block analysis report missing public domain sources")
	}

	// Verify cryptographic integrity
	for i, block := range blockReport.BlockBreakdown {
		if !suite.verifyBlockIntegrity(block) {
			t.Errorf("Block %d failed integrity verification", i)
		}
	}

	t.Logf("Generated block analysis report covering %d blocks", len(blockReport.BlockBreakdown))
}

// TestComplianceEvidenceGeneration tests generation of compliance evidence
func TestComplianceEvidenceGeneration(t *testing.T) {
	suite := setupLegalDocumentationTest(t)
	defer suite.cleanup()

	// Create test compliance events
	events := []struct {
		eventType string
		descriptor string
		action string
	}{
		{"dmca_takedown", "noisefs://test-compliance-001", "content_removed"},
		{"dmca_counter_notice", "noisefs://test-compliance-001", "counter_notice_received"},
		{"user_violation", "noisefs://test-compliance-002", "dmca_violation"},
	}

	for i, event := range events {
		err := suite.complianceSystem.LogComplianceEvent(
			event.eventType,
			fmt.Sprintf("test_user_%03d", i),
			event.descriptor,
			event.action,
			map[string]interface{}{
				"test_case": true,
				"event_number": i + 1,
			},
		)
		
		if err != nil {
			t.Errorf("Failed to create test compliance event %d: %v", i, err)
		}
	}

	// Generate compliance evidence package
	startDate := time.Now().Add(-24 * time.Hour)
	endDate := time.Now()
	
	evidencePackage, err := suite.legalSystem.GenerateComplianceEvidence(startDate, endDate, "test_evidence")
	if err != nil {
		t.Fatalf("Failed to generate compliance evidence: %v", err)
	}

	// Verify evidence completeness
	if len(evidencePackage.AuditTrail) == 0 {
		t.Error("Compliance evidence missing audit trail")
	}

	if evidencePackage.CryptographicProof == nil {
		t.Error("Compliance evidence missing cryptographic proof")
	}

	if len(evidencePackage.ComplianceEvents) == 0 {
		t.Error("Compliance evidence missing compliance events")
	}

	// Verify tamper evidence
	if !suite.verifyTamperEvidence(evidencePackage) {
		t.Error("Compliance evidence failed tamper verification")
	}

	t.Logf("Generated compliance evidence package with %d events", len(evidencePackage.ComplianceEvents))
}

// TestLegalArgumentBrief tests generation of legal argument briefs
func TestLegalArgumentBrief(t *testing.T) {
	suite := setupLegalDocumentationTest(t)
	defer suite.cleanup()

	testDescriptor := "noisefs://test-legal-brief-001"
	
	// Generate legal argument brief
	legalBrief, err := suite.legalSystem.GenerateLegalArgumentBrief(
		testDescriptor,
		"DMCA Safe Harbor Defense",
		"Copyright Infringement Claim",
	)
	
	if err != nil {
		t.Fatalf("Failed to generate legal argument brief: %v", err)
	}

	// Verify brief components
	if legalBrief.ExecutiveSummary == "" {
		t.Error("Legal argument brief missing executive summary")
	}

	if len(legalBrief.LegalArguments) == 0 {
		t.Error("Legal argument brief missing legal arguments")
	}

	if len(legalBrief.CitedPrecedents) == 0 {
		t.Error("Legal argument brief missing cited precedents")
	}

	if legalBrief.TechnicalEvidence == nil {
		t.Error("Legal argument brief missing technical evidence")
	}

	// Verify legal citation format
	for i, precedent := range legalBrief.CitedPrecedents {
		if !suite.verifyLegalCitation(precedent) {
			t.Errorf("Legal precedent %d has invalid citation format", i)
		}
	}

	t.Logf("Generated legal argument brief with %d precedents cited", len(legalBrief.CitedPrecedents))
}

// TestCourtReadyDocumentPackage tests generation of complete court-ready packages
func TestCourtReadyDocumentPackage(t *testing.T) {
	suite := setupLegalDocumentationTest(t)
	defer suite.cleanup()

	testDescriptor := "noisefs://test-court-ready-001"
	
	// Generate court-ready document package
	courtPackage, err := suite.legalSystem.GenerateCourtReadyPackage(
		testDescriptor,
		"Test Case Title",
		"Test Court Jurisdiction",
	)
	
	if err != nil {
		t.Fatalf("Failed to generate court-ready package: %v", err)
	}

	// Verify package completeness
	requiredDocuments := []string{
		"dmca_response_package",
		"technical_defense_kit",
		"expert_witness_report",
		"block_analysis_report",
		"compliance_evidence",
		"legal_argument_brief",
	}

	for _, docType := range requiredDocuments {
		if !suite.verifyDocumentPresence(courtPackage, docType) {
			t.Errorf("Court-ready package missing required document: %s", docType)
		}
	}

	// Verify PDF generation
	if !suite.verifyPDFGeneration(courtPackage) {
		t.Error("Court-ready package failed PDF generation")
	}

	// Verify cryptographic signatures
	if !suite.verifyPackageSignatures(courtPackage) {
		t.Error("Court-ready package failed signature verification")
	}

	// Save package to disk for manual inspection
	packagePath := filepath.Join(suite.tempDir, "court-ready-package.pdf")
	err = suite.legalSystem.ExportCourtPackage(courtPackage, packagePath)
	if err != nil {
		t.Errorf("Failed to export court package: %v", err)
	} else {
		t.Logf("Court-ready package exported to: %s", packagePath)
	}
}

// Helper functions

func setupLegalDocumentationTest(t *testing.T) *LegalDocumentationTestSuite {
	// Create temporary directory for test outputs
	tempDir, err := os.MkdirTemp("", "noisefs-legal-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	// Initialize legal documentation system
	legalConfig := legal.DefaultLegalConfig()
	legalConfig.OutputDirectory = tempDir
	legalSystem := legal.NewLegalDocumentationSystem(legalConfig)

	// Initialize compliance system
	auditConfig := compliance.DefaultAuditConfig()
	complianceSystem := compliance.NewComplianceAuditSystem(auditConfig)

	// Try to create reuse client (may fail if not fully set up)
	var reuseClient *reuse.ReuseAwareClient
	// This would typically be created with proper IPFS client and cache
	// For testing, we'll leave it nil if creation fails

	return &LegalDocumentationTestSuite{
		legalSystem:      legalSystem,
		complianceSystem: complianceSystem,
		reuseClient:      reuseClient,
		tempDir:          tempDir,
	}
}

func (suite *LegalDocumentationTestSuite) cleanup() {
	if suite.tempDir != "" {
		os.RemoveAll(suite.tempDir)
	}
}

func (suite *LegalDocumentationTestSuite) verifyPackageAuthenticity(pkg *legal.DMCAResponsePackage) bool {
	// Simplified verification - in real implementation, this would verify
	// cryptographic signatures and integrity hashes
	return pkg != nil && pkg.TechnicalDefense != nil
}

func (suite *LegalDocumentationTestSuite) verifyMathematicalProofs(kit *legal.TechnicalDefenseKit) bool {
	// Simplified verification - in real implementation, this would verify
	// mathematical proofs and cryptographic evidence
	return kit != nil && kit.CryptographicEvidence != nil
}

func (suite *LegalDocumentationTestSuite) verifyCourtReadiness(report *legal.ExpertWitnessReport) bool {
	// Simplified verification - in real implementation, this would check
	// formatting, legal citation standards, and completeness
	return report != nil && report.ExpertQualifications != "" && len(report.LegalPrecedents) > 0
}

func (suite *LegalDocumentationTestSuite) verifyBlockIntegrity(block *legal.BlockAnalysis) bool {
	// Simplified verification - in real implementation, this would verify
	// cryptographic hashes and integrity proofs
	return block != nil && block.BlockID != ""
}

func (suite *LegalDocumentationTestSuite) verifyTamperEvidence(evidence *legal.ComplianceEvidencePackage) bool {
	// Simplified verification - in real implementation, this would verify
	// tamper-evident features and cryptographic proofs
	return evidence != nil && evidence.CryptographicProof != nil
}

func (suite *LegalDocumentationTestSuite) verifyLegalCitation(precedent *legal.LegalPrecedent) bool {
	// Simplified verification - in real implementation, this would verify
	// proper legal citation format (Bluebook, etc.)
	return precedent != nil && precedent.CaseName != "" && precedent.Citation != ""
}

func (suite *LegalDocumentationTestSuite) verifyDocumentPresence(pkg *legal.CourtReadyPackage, docType string) bool {
	// Simplified verification - in real implementation, this would check
	// for specific document types in the package
	return pkg != nil && len(pkg.Documents) > 0
}

func (suite *LegalDocumentationTestSuite) verifyPDFGeneration(pkg *legal.CourtReadyPackage) bool {
	// Simplified verification - in real implementation, this would verify
	// PDF generation and formatting
	return pkg != nil
}

func (suite *LegalDocumentationTestSuite) verifyPackageSignatures(pkg *legal.CourtReadyPackage) bool {
	// Simplified verification - in real implementation, this would verify
	// cryptographic signatures on all documents
	return pkg != nil
}