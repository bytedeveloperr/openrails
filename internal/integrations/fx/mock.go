package fx

import (
	"context"
	"fmt"
	"time"
)

// MockProvider is a test double for FX rate provider.
type MockProvider struct {
	// Rates maps currency codes to USD rates.
	// e.g., {"eur": 1.08, "gbp": 1.27}
	Rates map[string]float64

	// Error, if set, will be returned for all calls.
	Error error

	// CallCount tracks how many times QuoteToUSD was called.
	CallCount int

	// LastCurrency records the last currency requested.
	LastCurrency string
}

// NewMockProvider creates a MockProvider with the given rates.
func NewMockProvider(rates map[string]float64) *MockProvider {
	if rates == nil {
		rates = make(map[string]float64)
	}
	// Always include USD
	rates["usd"] = 1.0
	return &MockProvider{
		Rates: rates,
	}
}

// QuoteToUSD returns a mock quote based on configured rates.
func (p *MockProvider) QuoteToUSD(ctx context.Context, currency string) (*Quote, error) {
	p.CallCount++
	p.LastCurrency = currency

	if p.Error != nil {
		return nil, p.Error
	}

	rate, ok := p.Rates[currency]
	if !ok {
		return nil, fmt.Errorf("unsupported currency: %s", currency)
	}

	return &Quote{
		FromCurrency: currency,
		ToCurrency:   "usd",
		Rate:         rate,
		AsOf:         time.Now(),
	}, nil
}

// SetRate sets the rate for a specific currency.
func (p *MockProvider) SetRate(currency string, rate float64) {
	p.Rates[currency] = rate
}

// Reset clears call tracking.
func (p *MockProvider) Reset() {
	p.CallCount = 0
	p.LastCurrency = ""
	p.Error = nil
}
