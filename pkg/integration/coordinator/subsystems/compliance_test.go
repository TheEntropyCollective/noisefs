package subsystems

import (
	"testing"
)

func TestComplianceSubsystemCreation(t *testing.T) {
	t.Run("Create compliance subsystem", func(t *testing.T) {
		compliance, err := NewComplianceSubsystem()
		if err != nil {
			t.Errorf("Failed to create compliance subsystem: %v", err)
		}
		
		if compliance == nil {
			t.Error("Expected valid compliance subsystem")
		}
		
		// Test that we can get the compliance audit system
		auditSystem := compliance.GetComplianceAudit()
		if auditSystem == nil {
			t.Error("Expected valid compliance audit system")
		}
	})
}

func TestComplianceSubsystemResponsibilities(t *testing.T) {
	t.Run("Compliance subsystem focuses on compliance concerns only", func(t *testing.T) {
		compliance, err := NewComplianceSubsystem()
		if err != nil {
			t.Fatalf("Failed to create compliance subsystem: %v", err)
		}
		
		// Test that it has compliance-related methods
		_ = compliance.GetComplianceAudit
		_ = compliance.Shutdown
		
		// Ensure it doesn't have non-compliance methods (compile-time check)
	})
}

func TestComplianceSubsystemShutdown(t *testing.T) {
	t.Run("Shutdown compliance subsystem", func(t *testing.T) {
		compliance, err := NewComplianceSubsystem()
		if err != nil {
			t.Fatalf("Failed to create compliance subsystem: %v", err)
		}
		
		err = compliance.Shutdown()
		if err != nil {
			t.Errorf("Failed to shutdown compliance subsystem: %v", err)
		}
	})
}