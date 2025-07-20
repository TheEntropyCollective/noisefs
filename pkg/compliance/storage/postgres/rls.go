package postgres

import (
	"context"
	"fmt"
)

// EnableRLSPolicies enables Row-Level Security policies for compliance tables
func (db *ComplianceDatabase) EnableRLSPolicies(ctx context.Context) error {
	// Enable RLS on tables
	tables := []string{
		"takedown_records",
		"violation_records", 
		"audit_entries",
		"notification_records",
	}

	for _, table := range tables {
		// Enable RLS
		query := fmt.Sprintf("ALTER TABLE %s ENABLE ROW LEVEL SECURITY", table)
		if _, err := db.pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to enable RLS on table %s: %w", table, err)
		}
	}

	// Create RLS policies
	if err := db.createRLSPolicies(ctx); err != nil {
		return fmt.Errorf("failed to create RLS policies: %w", err)
	}

	return nil
}

// createRLSPolicies creates the RLS policy definitions
func (db *ComplianceDatabase) createRLSPolicies(ctx context.Context) error {
	policies := []string{
		// Admin role can access all records
		`CREATE POLICY admin_all_access ON takedown_records 
		 FOR ALL TO admin 
		 USING (true)`,

		`CREATE POLICY admin_all_access ON violation_records 
		 FOR ALL TO admin 
		 USING (true)`,

		`CREATE POLICY admin_all_access ON audit_entries 
		 FOR ALL TO admin 
		 USING (true)`,

		`CREATE POLICY admin_all_access ON notification_records 
		 FOR ALL TO admin 
		 USING (true)`,

		// Legal role can read takedown records and related audit entries
		`CREATE POLICY legal_takedown_read ON takedown_records 
		 FOR SELECT TO legal 
		 USING (true)`,

		`CREATE POLICY legal_audit_read ON audit_entries 
		 FOR SELECT TO legal 
		 USING (event_type IN ('dmca_takedown', 'counter_notice', 'reinstatement'))`,

		// Legal role cannot read personal user violation records
		`CREATE POLICY legal_violation_restricted ON violation_records 
		 FOR SELECT TO legal 
		 USING (false)`, // Block access to personal violation data

		// User role can only access their own violation records
		`CREATE POLICY user_own_violations ON violation_records 
		 FOR SELECT TO user_role 
		 USING (user_id = current_setting('app.current_user_id', true))`,

		// User role can read their own notifications
		`CREATE POLICY user_own_notifications ON notification_records 
		 FOR SELECT TO user_role 
		 USING (target_user_id = current_setting('app.current_user_id', true))`,

		// Users cannot access takedown records or audit entries
		`CREATE POLICY user_no_takedown_access ON takedown_records 
		 FOR ALL TO user_role 
		 USING (false)`,

		`CREATE POLICY user_no_audit_access ON audit_entries 
		 FOR ALL TO user_role 
		 USING (false)`,

		// Users cannot create takedown records
		`CREATE POLICY user_no_takedown_insert ON takedown_records 
		 FOR INSERT TO user_role 
		 WITH CHECK (false)`,
	}

	for _, policy := range policies {
		// Drop policy if it exists (idempotent)
		dropQuery := fmt.Sprintf("DROP POLICY IF EXISTS %s", 
			extractPolicyName(policy))
		db.pool.Exec(ctx, dropQuery) // Ignore errors for non-existent policies

		// Create the policy
		if _, err := db.pool.Exec(ctx, policy); err != nil {
			return fmt.Errorf("failed to create RLS policy: %w", err)
		}
	}

	return nil
}

// extractPolicyName extracts policy name from CREATE POLICY statement
func extractPolicyName(policy string) string {
	// Simple extraction - find text between "CREATE POLICY " and " ON"
	start := len("CREATE POLICY ")
	end := 0
	for i := start; i < len(policy); i++ {
		if policy[i:i+3] == " ON" {
			end = i
			break
		}
	}
	if end == 0 {
		return ""
	}
	return policy[start:end]
}

// SetUserRole sets the user role and ID for the current database session
func (db *ComplianceDatabase) SetUserRole(ctx context.Context, role, userID string) context.Context {
	// Set session variables for RLS policies
	setRoleQuery := fmt.Sprintf("SET ROLE %s", role)
	db.pool.Exec(ctx, setRoleQuery) // Ignore errors

	setUserQuery := "SET app.current_user_id = $1"
	db.pool.Exec(ctx, setUserQuery, userID) // Ignore errors

	// Return context with role information for testing
	return context.WithValue(ctx, "user_role", role)
}

// CreateRoles creates the database roles for RLS
func (db *ComplianceDatabase) CreateRoles(ctx context.Context) error {
	roles := []string{
		"CREATE ROLE admin",
		"CREATE ROLE legal", 
		"CREATE ROLE user_role",
	}

	for _, roleSQL := range roles {
		// Try to create role, ignore if already exists
		if _, err := db.pool.Exec(ctx, roleSQL); err != nil {
			// Check if it's a "role already exists" error
			if !contains(err.Error(), "already exists") {
				return fmt.Errorf("failed to create role: %w", err)
			}
		}
	}

	return nil
}

// GrantPermissions grants necessary permissions to roles
func (db *ComplianceDatabase) GrantPermissions(ctx context.Context) error {
	permissions := []string{
		// Admin permissions
		"GRANT ALL ON ALL TABLES IN SCHEMA public TO admin",
		"GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO admin",

		// Legal permissions
		"GRANT SELECT ON takedown_records TO legal",
		"GRANT SELECT ON audit_entries TO legal",
		"GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO legal",

		// User permissions  
		"GRANT SELECT ON violation_records TO user_role",
		"GRANT SELECT ON notification_records TO user_role",
		"GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO user_role",
	}

	for _, permSQL := range permissions {
		if _, err := db.pool.Exec(ctx, permSQL); err != nil {
			return fmt.Errorf("failed to grant permission: %w", err)
		}
	}

	return nil
}

// DisableRLSPolicies disables RLS policies (for testing/maintenance)
func (db *ComplianceDatabase) DisableRLSPolicies(ctx context.Context) error {
	tables := []string{
		"takedown_records",
		"violation_records",
		"audit_entries", 
		"notification_records",
	}

	for _, table := range tables {
		query := fmt.Sprintf("ALTER TABLE %s DISABLE ROW LEVEL SECURITY", table)
		if _, err := db.pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to disable RLS on table %s: %w", table, err)
		}
	}

	return nil
}

// ResetUserRole resets the current user role to default
func (db *ComplianceDatabase) ResetUserRole(ctx context.Context) error {
	resetQueries := []string{
		"RESET ROLE",
		"RESET app.current_user_id",
	}

	for _, query := range resetQueries {
		db.pool.Exec(ctx, query) // Ignore errors
	}

	return nil
}

// TestRLSPolicy tests if RLS policy is working correctly
func (db *ComplianceDatabase) TestRLSPolicy(ctx context.Context, role, userID, testQuery string) (int, error) {
	// Set role context
	ctx = db.SetUserRole(ctx, role, userID)

	// Execute test query and count results
	var count int
	err := db.pool.QueryRow(ctx, testQuery).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to execute RLS test query: %w", err)
	}

	return count, nil
}