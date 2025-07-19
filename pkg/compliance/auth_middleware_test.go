package compliance

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Test JWT claims structure for NoiseFS compliance system
type JWTClaims struct {
	UserID   string   `json:"user_id"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Role     string   `json:"role"`     // "admin", "legal", "user"
	Scopes   []string `json:"scopes"`   // specific permissions
	Issuer   string   `json:"iss"`      // "noisefs-compliance"
	Audience string   `json:"aud"`      // "noisefs-api"
	IssuedAt int64    `json:"iat"`      // issued at timestamp
	ExpiresAt int64   `json:"exp"`      // expiration timestamp
	NotBefore int64   `json:"nbf"`      // not before timestamp
}

// Test JWT token structure
type TestJWT struct {
	Header    map[string]interface{} `json:"header"`
	Claims    JWTClaims              `json:"claims"`
	Signature string                 `json:"signature"`
}

// AuthenticationMiddleware represents the middleware that will be implemented
type AuthenticationMiddleware struct {
	secretKey []byte
	issuer    string
	audience  string
}

// AuthenticationContext represents authentication state for requests
type AuthenticationContext struct {
	UserID      string
	Username    string
	Email       string
	Role        string
	Scopes      []string
	Authenticated bool
	Token       string
}

// Test helper functions for JWT token generation
func generateTestJWT(claims JWTClaims, secretKey []byte) (string, error) {
	// Header
	header := map[string]interface{}{
		"typ": "JWT",
		"alg": "HS256",
	}
	
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	headerB64 := base64.RawURLEncoding.EncodeToString(headerBytes)
	
	// Claims
	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsBytes)
	
	// Signature
	message := headerB64 + "." + claimsB64
	h := hmac.New(sha256.New, secretKey)
	h.Write([]byte(message))
	signature := base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	
	return headerB64 + "." + claimsB64 + "." + signature, nil
}

func generateValidTestClaims(role string) JWTClaims {
	now := time.Now()
	return JWTClaims{
		UserID:    fmt.Sprintf("user-%d", now.Unix()),
		Username:  fmt.Sprintf("testuser_%s", role),
		Email:     fmt.Sprintf("test_%s@noisefs.test", role),
		Role:      role,
		Scopes:    getScopesForRole(role),
		Issuer:    "noisefs-compliance",
		Audience:  "noisefs-api",
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(time.Hour).Unix(),
		NotBefore: now.Unix(),
	}
}

func getScopesForRole(role string) []string {
	switch role {
	case "admin":
		return []string{"dmca:process", "audit:read", "audit:write", "users:read", "users:write", "reports:generate"}
	case "legal":
		return []string{"dmca:process", "audit:read", "reports:generate"}
	case "user":
		return []string{"profile:read", "violations:read:own"}
	default:
		return []string{}
	}
}

// TestAuthenticationMiddleware_ValidJWTToken tests valid JWT token scenarios
func TestAuthenticationMiddleware_ValidJWTToken(t *testing.T) {
	middleware := &AuthenticationMiddleware{
		secretKey: []byte("test-secret-key-for-compliance-auth"),
		issuer:    "noisefs-compliance",
		audience:  "noisefs-api",
	}
	
	testCases := []struct {
		name     string
		role     string
		expected bool
	}{
		{
			name:     "Valid Admin Token",
			role:     "admin",
			expected: true,
		},
		{
			name:     "Valid Legal Token",
			role:     "legal",
			expected: true,
		},
		{
			name:     "Valid User Token",
			role:     "user",
			expected: true,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate valid token
			claims := generateValidTestClaims(tc.role)
			token, err := generateTestJWT(claims, middleware.secretKey)
			if err != nil {
				t.Fatalf("Failed to generate test token: %v", err)
			}
			
			// Create test request
			req := httptest.NewRequest("GET", "/compliance/audit", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			
			// Test authentication (this will fail until implementation exists)
			authCtx, err := middleware.authenticateRequest(req)
			
			// TDD: This test should fail initially
			if tc.expected {
				if err != nil {
					t.Errorf("Expected successful authentication for %s, got error: %v", tc.role, err)
				}
				if authCtx == nil {
					t.Errorf("Expected authentication context for %s, got nil", tc.role)
				}
				if authCtx != nil && !authCtx.Authenticated {
					t.Errorf("Expected authenticated=true for %s, got false", tc.role)
				}
				if authCtx != nil && authCtx.Role != tc.role {
					t.Errorf("Expected role %s, got %s", tc.role, authCtx.Role)
				}
			}
		})
	}
}

// TestAuthenticationMiddleware_InvalidJWTToken tests invalid/malformed JWT tokens
func TestAuthenticationMiddleware_InvalidJWTToken(t *testing.T) {
	middleware := &AuthenticationMiddleware{
		secretKey: []byte("test-secret-key-for-compliance-auth"),
		issuer:    "noisefs-compliance",
		audience:  "noisefs-api",
	}
	
	testCases := []struct {
		name        string
		token       string
		expectedErr string
	}{
		{
			name:        "Malformed Token - Not Enough Parts",
			token:       "invalid.token",
			expectedErr: "malformed token",
		},
		{
			name:        "Malformed Token - Invalid Base64",
			token:       "invalid!!!.payload!!!.signature!!!",
			expectedErr: "invalid token encoding",
		},
		{
			name:        "Invalid JSON in Header",
			token:       base64.RawURLEncoding.EncodeToString([]byte("{invalid json")) + ".payload.signature",
			expectedErr: "invalid header format",
		},
		{
			name:        "Invalid Signature",
			token:       func() string {
				claims := generateValidTestClaims("admin")
				token, _ := generateTestJWT(claims, []byte("wrong-secret"))
				return token
			}(),
			expectedErr: "invalid signature",
		},
		{
			name:        "Empty Token",
			token:       "",
			expectedErr: "empty token",
		},
		{
			name:        "Token Without Bearer Prefix",
			token:       func() string {
				claims := generateValidTestClaims("admin")
				token, _ := generateTestJWT(claims, middleware.secretKey)
				return token
			}(),
			expectedErr: "invalid authorization format",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/compliance/audit", nil)
			
			if tc.name == "Token Without Bearer Prefix" {
				req.Header.Set("Authorization", tc.token)
			} else if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			
			// Test authentication (this will fail until implementation exists)
			authCtx, err := middleware.authenticateRequest(req)
			
			// TDD: These tests should fail initially, then pass when implementation is added
			if err == nil {
				t.Errorf("Expected authentication error for %s, got success", tc.name)
			}
			if authCtx != nil && authCtx.Authenticated {
				t.Errorf("Expected authentication failure for %s, got authenticated=true", tc.name)
			}
			
			// Check specific error message when implementation exists
			if err != nil && !strings.Contains(err.Error(), "not implemented") {
				if !strings.Contains(err.Error(), tc.expectedErr) {
					t.Errorf("Expected error containing '%s', got: %v", tc.expectedErr, err)
				}
			}
		})
	}
}

// TestAuthenticationMiddleware_ExpiredToken tests expired JWT tokens
func TestAuthenticationMiddleware_ExpiredToken(t *testing.T) {
	middleware := &AuthenticationMiddleware{
		secretKey: []byte("test-secret-key-for-compliance-auth"),
		issuer:    "noisefs-compliance",
		audience:  "noisefs-api",
	}
	
	// Generate expired token
	now := time.Now()
	expiredClaims := JWTClaims{
		UserID:    "user-expired",
		Username:  "expired_user",
		Email:     "expired@noisefs.test",
		Role:      "user",
		Scopes:    []string{"profile:read"},
		Issuer:    "noisefs-compliance",
		Audience:  "noisefs-api",
		IssuedAt:  now.Add(-2 * time.Hour).Unix(),
		ExpiresAt: now.Add(-1 * time.Hour).Unix(), // Expired 1 hour ago
		NotBefore: now.Add(-2 * time.Hour).Unix(),
	}
	
	token, err := generateTestJWT(expiredClaims, middleware.secretKey)
	if err != nil {
		t.Fatalf("Failed to generate expired token: %v", err)
	}
	
	// Create test request
	req := httptest.NewRequest("GET", "/compliance/audit", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	
	// Test authentication
	authCtx, err := middleware.authenticateRequest(req)
	
	// TDD: This test should fail initially
	if err == nil {
		t.Error("Expected authentication error for expired token, got success")
	}
	if authCtx != nil && authCtx.Authenticated {
		t.Error("Expected authentication failure for expired token, got authenticated=true")
	}
	
	// Check specific error message when implementation exists
	if err != nil && !strings.Contains(err.Error(), "not implemented") {
		if !strings.Contains(err.Error(), "token expired") {
			t.Errorf("Expected error containing 'token expired', got: %v", err)
		}
	}
}

// TestAuthenticationMiddleware_MissingAuthorizationHeader tests missing Authorization header
func TestAuthenticationMiddleware_MissingAuthorizationHeader(t *testing.T) {
	middleware := &AuthenticationMiddleware{
		secretKey: []byte("test-secret-key-for-compliance-auth"),
		issuer:    "noisefs-compliance",
		audience:  "noisefs-api",
	}
	
	// Create test request without Authorization header
	req := httptest.NewRequest("GET", "/compliance/audit", nil)
	
	// Test authentication
	authCtx, err := middleware.authenticateRequest(req)
	
	// TDD: This test should fail initially
	if err == nil {
		t.Error("Expected authentication error for missing Authorization header, got success")
	}
	if authCtx != nil && authCtx.Authenticated {
		t.Error("Expected authentication failure for missing header, got authenticated=true")
	}
	
	// Check specific error message when implementation exists
	if err != nil && !strings.Contains(err.Error(), "not implemented") {
		if !strings.Contains(err.Error(), "missing authorization header") {
			t.Errorf("Expected error containing 'missing authorization header', got: %v", err)
		}
	}
}

// TestAuthenticationMiddleware_MalformedAuthorizationHeader tests malformed Authorization headers
func TestAuthenticationMiddleware_MalformedAuthorizationHeader(t *testing.T) {
	middleware := &AuthenticationMiddleware{
		secretKey: []byte("test-secret-key-for-compliance-auth"),
		issuer:    "noisefs-compliance",
		audience:  "noisefs-api",
	}
	
	testCases := []struct {
		name   string
		header string
	}{
		{
			name:   "Wrong Scheme - Basic",
			header: "Basic dXNlcjpwYXNz",
		},
		{
			name:   "Wrong Scheme - Digest",
			header: "Digest username=\"test\"",
		},
		{
			name:   "No Scheme",
			header: "token-without-scheme",
		},
		{
			name:   "Multiple Tokens",
			header: "Bearer token1 token2",
		},
		{
			name:   "Bearer Without Token",
			header: "Bearer",
		},
		{
			name:   "Bearer With Empty Token",
			header: "Bearer ",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/compliance/audit", nil)
			req.Header.Set("Authorization", tc.header)
			
			// Test authentication
			authCtx, err := middleware.authenticateRequest(req)
			
			// TDD: These tests should fail initially
			if err == nil {
				t.Errorf("Expected authentication error for %s, got success", tc.name)
			}
			if authCtx != nil && authCtx.Authenticated {
				t.Errorf("Expected authentication failure for %s, got authenticated=true", tc.name)
			}
		})
	}
}

// TestAuthenticationMiddleware_TokenRefresh tests token refresh scenarios
func TestAuthenticationMiddleware_TokenRefresh(t *testing.T) {
	middleware := &AuthenticationMiddleware{
		secretKey: []byte("test-secret-key-for-compliance-auth"),
		issuer:    "noisefs-compliance",
		audience:  "noisefs-api",
	}
	
	// Generate token that's about to expire (within refresh window)
	now := time.Now()
	soonToExpireClaims := JWTClaims{
		UserID:    "user-refresh",
		Username:  "refresh_user",
		Email:     "refresh@noisefs.test",
		Role:      "legal",
		Scopes:    []string{"dmca:process", "audit:read"},
		Issuer:    "noisefs-compliance",
		Audience:  "noisefs-api",
		IssuedAt:  now.Add(-55 * time.Minute).Unix(),
		ExpiresAt: now.Add(5 * time.Minute).Unix(), // Expires in 5 minutes
		NotBefore: now.Add(-55 * time.Minute).Unix(),
	}
	
	token, err := generateTestJWT(soonToExpireClaims, middleware.secretKey)
	if err != nil {
		t.Fatalf("Failed to generate soon-to-expire token: %v", err)
	}
	
	// Create test request
	req := httptest.NewRequest("GET", "/compliance/audit", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	
	// Test authentication
	authCtx, err := middleware.authenticateRequest(req)
	
	// TDD: This test should fail initially
	// The implementation should detect tokens near expiry and suggest refresh
	if err == nil {
		t.Error("Expected authentication warning/error for soon-to-expire token")
	}
	
	// When implemented, should return context with refresh warning
	if authCtx != nil {
		// Implementation should include refresh recommendation
		t.Log("Token refresh scenario - implementation should handle near-expiry tokens")
	}
}

// Stub implementation to satisfy compilation - will fail all tests initially (TDD)
func (m *AuthenticationMiddleware) authenticateRequest(req *http.Request) (*AuthenticationContext, error) {
	return nil, fmt.Errorf("authentication middleware not implemented")
}

// TestHelper functions for other test files

// CreateTestAuthContext creates a test authentication context for integration tests
func CreateTestAuthContext(role string) *AuthenticationContext {
	return &AuthenticationContext{
		UserID:      fmt.Sprintf("test-user-%s", role),
		Username:    fmt.Sprintf("testuser_%s", role),
		Email:       fmt.Sprintf("test_%s@noisefs.test", role),
		Role:        role,
		Scopes:      getScopesForRole(role),
		Authenticated: true,
		Token:       "test-token",
	}
}

// CreateTestJWTToken creates a valid test JWT token for integration testing
func CreateTestJWTToken(role string) (string, error) {
	secretKey := []byte("test-secret-key-for-compliance-auth")
	claims := generateValidTestClaims(role)
	return generateTestJWT(claims, secretKey)
}

// CreateExpiredTestJWTToken creates an expired test JWT token for negative testing
func CreateExpiredTestJWTToken(role string) (string, error) {
	secretKey := []byte("test-secret-key-for-compliance-auth")
	now := time.Now()
	expiredClaims := JWTClaims{
		UserID:    fmt.Sprintf("expired-user-%s", role),
		Username:  fmt.Sprintf("expired_%s", role),
		Email:     fmt.Sprintf("expired_%s@noisefs.test", role),
		Role:      role,
		Scopes:    getScopesForRole(role),
		Issuer:    "noisefs-compliance",
		Audience:  "noisefs-api",
		IssuedAt:  now.Add(-2 * time.Hour).Unix(),
		ExpiresAt: now.Add(-1 * time.Hour).Unix(), // Expired
		NotBefore: now.Add(-2 * time.Hour).Unix(),
	}
	return generateTestJWT(expiredClaims, secretKey)
}