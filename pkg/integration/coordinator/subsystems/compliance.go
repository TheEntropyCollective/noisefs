package subsystems

import (
	"github.com/TheEntropyCollective/noisefs/pkg/compliance"
)

// ComplianceSubsystem manages all compliance-related components
type ComplianceSubsystem struct {
	complianceAudit *compliance.ComplianceAuditSystem
}

// NewComplianceSubsystem creates a new compliance subsystem
func NewComplianceSubsystem() (*ComplianceSubsystem, error) {
	subsystem := &ComplianceSubsystem{}

	if err := subsystem.initializeCompliance(); err != nil {
		return nil, err
	}

	return subsystem, nil
}

// GetComplianceAudit returns the compliance audit system
func (c *ComplianceSubsystem) GetComplianceAudit() *compliance.ComplianceAuditSystem {
	return c.complianceAudit
}

// initializeCompliance sets up legal compliance components
func (c *ComplianceSubsystem) initializeCompliance() error {
	auditConfig := compliance.DefaultAuditConfig()
	// Use default database path and retention period

	c.complianceAudit = compliance.NewComplianceAuditSystem(auditConfig)

	// Compliance audit system is ready to use

	return nil
}

// Shutdown gracefully shuts down the compliance subsystem
func (c *ComplianceSubsystem) Shutdown() error {
	// Compliance components cleanup would go here
	return nil
}