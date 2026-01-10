package solana

import (
	"context"
	"fmt"
	"strings"
	"time"

	solanago "github.com/doujins-org/solana-go"
	"github.com/doujins-org/solana-go/rpc"
	log "github.com/sirupsen/logrus"
)

// RPCClient handles interactions with the Solana blockchain
type RPCClient struct {
	client   *rpc.Client
	endpoint string
	network  string // "mainnet", "devnet", "testnet"
}

// NewRPCClient creates a new Solana RPC client
func NewRPCClient(endpoint, network string) *RPCClient {
	if endpoint == "" {
		switch network {
		case "mainnet":
			endpoint = "https://api.mainnet-beta.solana.com"
		case "devnet":
			endpoint = "https://api.devnet.solana.com"
		case "testnet":
			endpoint = "https://api.testnet.solana.com"
		default:
			endpoint = "https://api.devnet.solana.com"
			network = "devnet"
		}
	}

	client := rpc.New(endpoint)
	log.WithFields(log.Fields{
		"endpoint": endpoint,
		"network":  network,
	}).Info("Initialized Solana RPC client")

	return &RPCClient{
		client:   client,
		endpoint: endpoint,
		network:  network,
	}
}

// GetBalance returns the SOL balance for an address.
func (c *RPCClient) GetBalance(ctx context.Context, address solanago.PublicKey) (uint64, error) {
	balance, err := c.client.GetBalance(ctx, address, rpc.CommitmentFinalized)
	if err != nil {
		return 0, fmt.Errorf("failed to get balance for %s: %w", address.String(), err)
	}
	return balance.Value, nil
}

// GetTokenBalance returns the SPL token balance for an address and mint
func (c *RPCClient) GetTokenBalance(ctx context.Context, tokenAccount solanago.PublicKey) (*rpc.UiTokenAmount, error) {
	resp, err := c.client.GetTokenAccountBalance(ctx, tokenAccount, rpc.CommitmentFinalized)
	if err != nil {
		return nil, fmt.Errorf("failed to get token balance for %s: %w", tokenAccount.String(), err)
	}
	return resp.Value, nil
}

// SimulateTransaction simulates a transaction to check if it would succeed
func (c *RPCClient) SimulateTransaction(ctx context.Context, tx *solanago.Transaction) (*rpc.SimulateTransactionResponse, error) {
	resp, err := c.client.SimulateTransaction(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to simulate transaction: %w", err)
	}
	return resp, nil
}

// SendTransaction submits a transaction to the blockchain
func (c *RPCClient) SendTransaction(ctx context.Context, tx *solanago.Transaction) (solanago.Signature, error) {
	sig, err := c.client.SendTransaction(ctx, tx)
	if err != nil {
		return solanago.Signature{}, fmt.Errorf("failed to send transaction: %w", err)
	}

	log.WithFields(log.Fields{
		"signature": sig.String(),
		"network":   c.network,
	}).Info("Transaction sent to Solana")

	return sig, nil
}

// GetTransaction retrieves transaction details by signature
func (c *RPCClient) GetTransaction(ctx context.Context, signature solanago.Signature) (*rpc.GetTransactionResult, error) {
	resp, err := c.client.GetTransaction(ctx, signature, &rpc.GetTransactionOpts{
		Commitment: rpc.CommitmentConfirmed,
		Encoding:   solanago.EncodingBase64,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction %s: %w", signature.String(), err)
	}
	return resp, nil
}

func (c *RPCClient) GetTransactionWithRetry(ctx context.Context, signature solanago.Signature, attempts int, delay time.Duration) (*rpc.GetTransactionResult, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		resp, err := c.GetTransaction(ctx, signature)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !isNotFoundError(err) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, lastErr
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

// ConfirmTransaction waits for a transaction to be confirmed
func (c *RPCClient) ConfirmTransaction(ctx context.Context, signature solanago.Signature, commitment rpc.CommitmentType) error {
	timeout := 60 * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("transaction confirmation timeout for %s", signature.String())
		case <-ticker.C:
			status, err := c.client.GetSignatureStatuses(ctx, true, signature)
			if err != nil {
				log.WithError(err).Warn("Failed to get signature status")
				continue
			}

			if len(status.Value) > 0 && status.Value[0] != nil {
				sigStatus := status.Value[0]

				// Check for errors
				if sigStatus.Err != nil {
					return fmt.Errorf("transaction failed: %v", sigStatus.Err)
				}

				// Check if confirmed at desired level
				if sigStatus.ConfirmationStatus != "" {
					switch commitment {
					case rpc.CommitmentProcessed:
						if sigStatus.ConfirmationStatus == rpc.ConfirmationStatusProcessed ||
							sigStatus.ConfirmationStatus == rpc.ConfirmationStatusConfirmed ||
							sigStatus.ConfirmationStatus == rpc.ConfirmationStatusFinalized {
							return nil
						}
					case rpc.CommitmentConfirmed:
						if sigStatus.ConfirmationStatus == rpc.ConfirmationStatusConfirmed ||
							sigStatus.ConfirmationStatus == rpc.ConfirmationStatusFinalized {
							return nil
						}
					case rpc.CommitmentFinalized:
						if sigStatus.ConfirmationStatus == rpc.ConfirmationStatusFinalized {
							return nil
						}
					}
				}
			}
		}
	}
}

// GetLatestBlockhash gets the latest blockhash for transaction creation
func (c *RPCClient) GetLatestBlockhash(ctx context.Context) (solanago.Hash, error) {
	resp, err := c.client.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return solanago.Hash{}, fmt.Errorf("failed to get latest blockhash: %w", err)
	}
	return resp.Value.Blockhash, nil
}

// GetMinimumBalanceForRentExemption returns the minimum balance needed for rent exemption
func (c *RPCClient) GetMinimumBalanceForRentExemption(ctx context.Context, dataSize uint64) (uint64, error) {
	balance, err := c.client.GetMinimumBalanceForRentExemption(ctx, dataSize, rpc.CommitmentFinalized)
	if err != nil {
		return 0, fmt.Errorf("failed to get minimum balance for rent exemption: %w", err)
	}
	return balance, nil
}

// IsValidAddress checks if a public key string is valid
func (c *RPCClient) IsValidAddress(address string) bool {
	_, err := solanago.PublicKeyFromBase58(address)
	return err == nil
}

// ParseAddress converts a base58 string to PublicKey
func (c *RPCClient) ParseAddress(address string) (solanago.PublicKey, error) {
	return solanago.PublicKeyFromBase58(address)
}

// GetNetwork returns the current network
func (c *RPCClient) GetNetwork() string {
	return c.network
}

// GetEndpoint returns the current RPC endpoint
func (c *RPCClient) GetEndpoint() string {
	return c.endpoint
}

// SignatureInfo is a pared-down view of a signature lookup.
type SignatureInfo struct {
	Signature string
	HasError  bool
}

// GetSignaturesForAddress finds transactions that reference a specific address.
func (c *RPCClient) GetSignaturesForAddress(ctx context.Context, address string, limit int) ([]SignatureInfo, error) {
	pubkey, err := solanago.PublicKeyFromBase58(strings.TrimSpace(address))
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	opts := &rpc.GetSignaturesForAddressOpts{
		Commitment: rpc.CommitmentFinalized,
	}
	if limit > 0 {
		limitVal := limit
		opts.Limit = &limitVal
	}

	resp, err := c.client.GetSignaturesForAddressWithOpts(ctx, pubkey, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get signatures for address %s: %w", pubkey.String(), err)
	}

	results := make([]SignatureInfo, 0, len(resp))
	for _, sig := range resp {
		results = append(results, SignatureInfo{
			Signature: sig.Signature.String(),
			HasError:  sig.Err != nil,
		})
	}

	return results, nil
}

// GetClient returns the underlying RPC client for direct access when needed
func (c *RPCClient) GetClient() *rpc.Client {
	return c.client
}
