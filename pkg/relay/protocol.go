package relay

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"golang.org/x/crypto/nacl/box"
)

const (
	// Protocol IDs
	RelayProtocolID = protocol.ID("/noisefs/relay/1.0.0")
	
	// Message types
	MsgTypeBlockRequest   = "block_request"
	MsgTypeBlockResponse  = "block_response"
	MsgTypeCoverRequest   = "cover_request"
	MsgTypeHealthCheck    = "health_check"
	MsgTypeError          = "error"
)

// RelayProtocol handles encrypted communication between clients and relays
type RelayProtocol struct {
	keyPair   *KeyPair
	peerKeys  map[peer.ID]*[32]byte // Public keys of known peers
	nonces    map[string]*[24]byte  // Nonces for requests
}

// KeyPair represents a public/private key pair for encryption
type KeyPair struct {
	PublicKey  *[32]byte
	PrivateKey *[32]byte
}

// RelayMessage is the base message structure for relay communication
type RelayMessage struct {
	Type      string      `json:"type"`
	ID        string      `json:"id"`
	Timestamp time.Time   `json:"timestamp"`
	Encrypted []byte      `json:"encrypted,omitempty"`
	Payload   interface{} `json:"payload,omitempty"`
}

// BlockRequestPayload contains the payload for block requests
type BlockRequestPayload struct {
	BlockID    string            `json:"block_id"`
	RelayPath  []peer.ID         `json:"relay_path,omitempty"`
	Options    map[string]string `json:"options,omitempty"`
	PeerHint   peer.ID           `json:"peer_hint,omitempty"`
	Priority   int               `json:"priority,omitempty"`
	IsDecoy    bool              `json:"is_decoy,omitempty"`
}

// BlockResponsePayload contains the payload for block responses
type BlockResponsePayload struct {
	BlockID   string    `json:"block_id"`
	Data      []byte    `json:"data,omitempty"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	Latency   int64     `json:"latency"` // Milliseconds
	RelayID   peer.ID   `json:"relay_id"`
	Timestamp time.Time `json:"timestamp"`
}

// CoverRequestPayload contains the payload for cover traffic requests
type CoverRequestPayload struct {
	PopularBlocks []string `json:"popular_blocks"`
	Count         int      `json:"count"`
	Delay         int      `json:"delay"` // Milliseconds
}

// HealthCheckPayload contains the payload for health checks
type HealthCheckPayload struct {
	Timestamp time.Time `json:"timestamp"`
	TestBlock string    `json:"test_block,omitempty"`
}

// ErrorPayload contains error information
type ErrorPayload struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// NewRelayProtocol creates a new relay protocol instance
func NewRelayProtocol() (*RelayProtocol, error) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}
	
	return &RelayProtocol{
		keyPair:  keyPair,
		peerKeys: make(map[peer.ID]*[32]byte),
		nonces:   make(map[string]*[24]byte),
	}, nil
}

// GenerateKeyPair generates a new public/private key pair
func GenerateKeyPair() (*KeyPair, error) {
	publicKey, privateKey, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	
	return &KeyPair{
		PublicKey:  publicKey,
		PrivateKey: privateKey,
	}, nil
}

// AddPeerKey adds a public key for a peer
func (rp *RelayProtocol) AddPeerKey(peerID peer.ID, publicKey *[32]byte) {
	rp.peerKeys[peerID] = publicKey
}

// CreateBlockRequest creates an encrypted block request message
func (rp *RelayProtocol) CreateBlockRequest(ctx context.Context, blockID string, relayID peer.ID, options map[string]string) (*RelayMessage, error) {
	payload := &BlockRequestPayload{
		BlockID:  blockID,
		Options:  options,
		Priority: 1,
		IsDecoy:  false,
	}
	
	return rp.createEncryptedMessage(MsgTypeBlockRequest, payload, relayID)
}

// CreateCoverRequest creates an encrypted cover traffic request
func (rp *RelayProtocol) CreateCoverRequest(ctx context.Context, popularBlocks []string, count int, relayID peer.ID) (*RelayMessage, error) {
	payload := &CoverRequestPayload{
		PopularBlocks: popularBlocks,
		Count:         count,
		Delay:         0,
	}
	
	return rp.createEncryptedMessage(MsgTypeCoverRequest, payload, relayID)
}

// CreateHealthCheck creates a health check message
func (rp *RelayProtocol) CreateHealthCheck(ctx context.Context, testBlock string, relayID peer.ID) (*RelayMessage, error) {
	payload := &HealthCheckPayload{
		Timestamp: time.Now(),
		TestBlock: testBlock,
	}
	
	return rp.createEncryptedMessage(MsgTypeHealthCheck, payload, relayID)
}

// CreateBlockResponse creates a block response message
func (rp *RelayProtocol) CreateBlockResponse(ctx context.Context, blockID string, data []byte, success bool, errorMsg string, relayID peer.ID, clientID peer.ID) (*RelayMessage, error) {
	payload := &BlockResponsePayload{
		BlockID:   blockID,
		Data:      data,
		Success:   success,
		Error:     errorMsg,
		Latency:   0, // Will be calculated by client
		RelayID:   relayID,
		Timestamp: time.Now(),
	}
	
	return rp.createEncryptedMessage(MsgTypeBlockResponse, payload, clientID)
}

// CreateError creates an error message
func (rp *RelayProtocol) CreateError(ctx context.Context, code int, message string, details string, targetID peer.ID) (*RelayMessage, error) {
	payload := &ErrorPayload{
		Code:    code,
		Message: message,
		Details: details,
	}
	
	return rp.createEncryptedMessage(MsgTypeError, payload, targetID)
}

// DecryptMessage decrypts a received message
func (rp *RelayProtocol) DecryptMessage(msg *RelayMessage, senderID peer.ID) (interface{}, error) {
	if msg.Encrypted == nil {
		return msg.Payload, nil
	}
	
	// Get sender's public key
	senderPublicKey, exists := rp.peerKeys[senderID]
	if !exists {
		return nil, fmt.Errorf("no public key for sender %s", senderID)
	}
	
	// Get nonce for this message
	nonce, exists := rp.nonces[msg.ID]
	if !exists {
		return nil, fmt.Errorf("no nonce found for message %s", msg.ID)
	}
	
	// Decrypt the message
	decrypted, ok := box.Open(nil, msg.Encrypted, nonce, senderPublicKey, rp.keyPair.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("failed to decrypt message")
	}
	
	// Parse the decrypted payload based on message type
	var payload interface{}
	switch msg.Type {
	case MsgTypeBlockRequest:
		payload = &BlockRequestPayload{}
	case MsgTypeBlockResponse:
		payload = &BlockResponsePayload{}
	case MsgTypeCoverRequest:
		payload = &CoverRequestPayload{}
	case MsgTypeHealthCheck:
		payload = &HealthCheckPayload{}
	case MsgTypeError:
		payload = &ErrorPayload{}
	default:
		return nil, fmt.Errorf("unknown message type: %s", msg.Type)
	}
	
	err := json.Unmarshal(decrypted, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	
	return payload, nil
}

// createEncryptedMessage creates an encrypted message
func (rp *RelayProtocol) createEncryptedMessage(msgType string, payload interface{}, targetID peer.ID) (*RelayMessage, error) {
	// Generate message ID
	messageID := rp.generateMessageID()
	
	// Get target's public key
	targetPublicKey, exists := rp.peerKeys[targetID]
	if !exists {
		return nil, fmt.Errorf("no public key for target %s", targetID)
	}
	
	// Serialize payload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	
	// Generate nonce
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	
	// Store nonce for later decryption
	rp.nonces[messageID] = &nonce
	
	// Encrypt the payload
	encrypted := box.Seal(nil, payloadBytes, &nonce, targetPublicKey, rp.keyPair.PrivateKey)
	
	return &RelayMessage{
		Type:      msgType,
		ID:        messageID,
		Timestamp: time.Now(),
		Encrypted: encrypted,
	}, nil
}

// generateMessageID generates a unique message ID
func (rp *RelayProtocol) generateMessageID() string {
	// Generate random bytes
	randomBytes := make([]byte, 16)
	rand.Read(randomBytes)
	
	// Hash with timestamp
	hash := sha256.Sum256(append(randomBytes, []byte(time.Now().String())...))
	
	return fmt.Sprintf("%x", hash[:8])
}

// ValidateMessage validates a received message
func (rp *RelayProtocol) ValidateMessage(msg *RelayMessage) error {
	if msg.Type == "" {
		return fmt.Errorf("message type is required")
	}
	
	if msg.ID == "" {
		return fmt.Errorf("message ID is required")
	}
	
	if msg.Timestamp.IsZero() {
		return fmt.Errorf("message timestamp is required")
	}
	
	// Check if message is too old (replay protection)
	if time.Since(msg.Timestamp) > 5*time.Minute {
		return fmt.Errorf("message is too old")
	}
	
	// Check if we've seen this message ID before (replay protection)
	if _, exists := rp.nonces[msg.ID]; exists {
		return fmt.Errorf("duplicate message ID")
	}
	
	return nil
}

// GetPublicKey returns the public key for this protocol instance
func (rp *RelayProtocol) GetPublicKey() *[32]byte {
	return rp.keyPair.PublicKey
}

// CleanupNonces removes old nonces to prevent memory leaks
func (rp *RelayProtocol) CleanupNonces() {
	cutoff := time.Now().Add(-10 * time.Minute)
	
	for id, _ := range rp.nonces {
		// In a real implementation, you'd store the timestamp with the nonce
		// For now, we'll just clean up randomly
		if len(rp.nonces) > 1000 {
			delete(rp.nonces, id)
		}
	}
}