package solana

import (
	"fmt"

	solanago "github.com/doujins-org/solana-go"
)

// GenerateReference creates a new random public key and returns it as a base58 string.
// The private key is discarded.
func GenerateReference() (string, error) {
	wallet := solanago.NewWallet()
	if wallet == nil {
		return "", fmt.Errorf("failed to generate solana wallet")
	}
	return wallet.PublicKey().String(), nil
}
