package services

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/solana-go"
	log "github.com/sirupsen/logrus"
)

// SolanaVerificationService handles wallet ownership verification
type SolanaVerificationService struct {
	db         *db.DB
	rpc        *SolanaRPCService
	challenges map[string]*VerificationChallenge // In-memory store for challenges
}

// VerificationChallenge represents a challenge for wallet verification
type VerificationChallenge struct {
	UserID    string
	Address   string
	Message   string
	ExpiresAt time.Time
	Nonce     string
}

// NewSolanaVerificationService creates a new verification service
func NewSolanaVerificationService(db *db.DB, rpc *SolanaRPCService) *SolanaVerificationService {
	return &SolanaVerificationService{
		db:         db,
		rpc:        rpc,
		challenges: make(map[string]*VerificationChallenge),
	}
}

// GenerateChallenge creates a verification challenge for a wallet
func (s *SolanaVerificationService) GenerateChallenge(ctx context.Context, userID, address string) (*VerificationChallenge, error) {
	// Validate the address format
	pubKey, err := solana.PublicKeyFromBase58(address)
	if err != nil {
		return nil, fmt.Errorf("invalid Solana address: %w", err)
	}

	// Generate a random nonce
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	nonce := hex.EncodeToString(nonceBytes)

	// Create challenge message
	timestamp := time.Now().Unix()
	message := fmt.Sprintf("Verify wallet ownership for Doujins billing system.\nWallet: %s\nUser: %s\nNonce: %s\nTimestamp: %d",
		pubKey.String(), userID, nonce, timestamp)

	challenge := &VerificationChallenge{
		UserID:    userID,
		Address:   pubKey.String(),
		Message:   message,
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Nonce:     nonce,
	}

	// Store challenge (in production, this should be in Redis or database)
	challengeKey := fmt.Sprintf("%s:%s", userID, address)
	s.challenges[challengeKey] = challenge

	log.WithFields(log.Fields{
		"user_id": userID,
		"address": address,
		"nonce":   nonce,
	}).Info("Generated wallet verification challenge")

	return challenge, nil
}

// VerifySignature verifies a signed challenge and marks wallet as verified
func (s *SolanaVerificationService) VerifySignature(ctx context.Context, userID, address, signature, message string) error {
	// Get challenge
	challengeKey := fmt.Sprintf("%s:%s", userID, address)
	challenge, exists := s.challenges[challengeKey]
	if !exists {
		return fmt.Errorf("no challenge found for user %s and address %s", userID, address)
	}

	// Check if challenge expired
	if time.Now().After(challenge.ExpiresAt) {
		delete(s.challenges, challengeKey)
		return fmt.Errorf("challenge expired")
	}

	// Verify the message matches the challenge
	if message != challenge.Message {
		return fmt.Errorf("message does not match challenge")
	}

	// Parse the public key
	pubKey, err := solana.PublicKeyFromBase58(address)
	if err != nil {
		return fmt.Errorf("invalid address format: %w", err)
	}

	// Decode the signature
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("invalid signature format: %w", err)
	}

	// Verify Ed25519 signature
	if !ed25519.Verify(pubKey[:], []byte(message), sigBytes) {
		return fmt.Errorf("signature verification failed")
	}

	// Mark wallet as verified in database
	now := time.Now()
	_, err = s.db.GetDB().NewUpdate().Model((*models.SolanaWallet)(nil)).
		Set("is_verified = ?", true).
		Set("verified_at = ?", &now).
		Set("updated_at = ?", now).
		Where("user_id = ? AND address = ?", userID, address).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update wallet verification status: %w", err)
	}

	// Remove challenge
	delete(s.challenges, challengeKey)

	log.WithFields(log.Fields{
		"user_id": userID,
		"address": address,
	}).Info("Wallet successfully verified")

	return nil
}

// IsWalletVerified checks if a wallet is verified for a user
func (s *SolanaVerificationService) IsWalletVerified(ctx context.Context, userID, address string) (bool, error) {
	var wallet models.SolanaWallet
	err := s.db.GetDB().NewSelect().Model(&wallet).
		Where("user_id = ? AND address = ? AND is_verified = ?", userID, address, true).
		Scan(ctx)
	if err != nil {
		return false, nil // Not found or error means not verified
	}
	return true, nil
}

// GetChallenge retrieves an active challenge
func (s *SolanaVerificationService) GetChallenge(userID, address string) (*VerificationChallenge, bool) {
	challengeKey := fmt.Sprintf("%s:%s", userID, address)
	challenge, exists := s.challenges[challengeKey]
	if !exists {
		return nil, false
	}

	// Check if expired
	if time.Now().After(challenge.ExpiresAt) {
		delete(s.challenges, challengeKey)
		return nil, false
	}

	return challenge, true
}

// CleanupExpiredChallenges removes expired challenges
func (s *SolanaVerificationService) CleanupExpiredChallenges() {
	now := time.Now()
	for key, challenge := range s.challenges {
		if now.After(challenge.ExpiresAt) {
			delete(s.challenges, key)
		}
	}
}

// StartChallengeCleanup starts a goroutine to periodically clean up expired challenges
func (s *SolanaVerificationService) StartChallengeCleanup() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			s.CleanupExpiredChallenges()
		}
	}()
}
