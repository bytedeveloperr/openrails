package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Product represents a product offering (e.g., Premium Membership)
// This represents our product catalog concept
type Product struct {
	bun.BaseModel `bun:"table:billing.products,alias:prod"`

	ID          uuid.UUID `bun:"id,pk,type:uuid" json:"id"`
	Slug        string    `bun:"slug,notnull,unique" json:"slug"`
	DisplayName string    `bun:"display_name,notnull" json:"display_name"`
	Description string    `bun:"description,nullzero" json:"description"`

	// Entitlements configuration: map entitlement name -> duration days (nil or 0 means indefinite)
	EntitlementsSpec map[string]*int `bun:"entitlements_spec,type:jsonb,nullzero" json:"entitlements_spec,omitempty"`

	IsActive  bool      `bun:"is_active,notnull,default:true" json:"is_active"`
	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`

	// Relationships
	Prices []*Price `bun:"rel:has-many,join:id=product_id" json:"prices,omitempty"`
}

// Price represents a specific pricing option for a product
// This represents pricing options similar to Stripe's pricing model
type Price struct {
	bun.BaseModel `bun:"table:billing.prices,alias:price"`

	ID          uuid.UUID `bun:"id,pk,type:uuid" json:"id"`
	ProductID   uuid.UUID `bun:"product_id,notnull" json:"product_id"`
	DisplayName string    `bun:"display_name,notnull" json:"display_name"`
	IsActive    bool      `bun:"is_active,notnull,default:true" json:"is_active"`
	Amount      int64     `bun:"amount,notnull" json:"amount"`
	Currency    string    `bun:"currency,notnull" json:"currency"`

	// Billing interval in days (nullable for one-time purchases)
	// 30 = monthly, 365 = yearly, null = one-time purchase
	BillingCycleDays *int `bun:"billing_cycle_days,nullzero" json:"billing_cycle_days"`

	// Processors is a JSONB map of processor name -> processor-specific configuration
	// Keys: "nmi", "ccbill", "solana", etc.
	// Values: processor-specific data (e.g., plan_id, price_id, provider)
	// Example: {"nmi": {"plan_id": "123", "provider": "mobius"}, "ccbill": {"price_id": "456"}}
	Processors map[string]map[string]string `bun:"processors,type:jsonb,nullzero" json:"processors,omitempty"`

	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`

	// Relationships
	Product       *Product       `bun:"rel:belongs-to,join:product_id=id" json:"-"`
	Subscriptions []Subscription `bun:"rel:has-many,join:id=price_id" json:"-"`
	Payments      []Payment      `bun:"rel:has-many,join:id=price_id" json:"-"`
}

// Processor config key constants (used in the Processors JSONB map)
const (
	ProcessorKeyPlanID   = "plan_id"
	ProcessorKeyPriceID  = "price_id"
	ProcessorKeyProvider = "provider"
)

// GetProcessorConfig returns the configuration for a specific processor, or nil if not configured
func (p *Price) GetProcessorConfig(processor Processor) map[string]string {
	if p.Processors == nil {
		return nil
	}
	return p.Processors[string(processor)]
}

// HasProcessor checks if a specific processor is configured for this price
func (p *Price) HasProcessor(processor Processor) bool {
	return p.GetProcessorConfig(processor) != nil
}

// GetNMIConfig returns the NMI processor configuration
func (p *Price) GetNMIConfig() (planID, provider string, ok bool) {
	config := p.GetProcessorConfig(ProcessorNMI)
	if config == nil {
		return "", "", false
	}
	planID = config[ProcessorKeyPlanID]
	provider = config[ProcessorKeyProvider]
	if provider == "" {
		provider = "mobius" // default provider
	}
	return planID, provider, planID != ""
}

// GetCCBillConfig returns the CCBill processor configuration
func (p *Price) GetCCBillConfig() (priceID string, ok bool) {
	config := p.GetProcessorConfig(ProcessorCCBill)
	if config == nil {
		return "", false
	}
	priceID = config[ProcessorKeyPriceID]
	return priceID, priceID != ""
}

// GetSolanaConfig returns the Solana processor configuration
func (p *Price) GetSolanaConfig() (ok bool) {
	// Solana processor just needs to be present in the map to be enabled
	return p.HasProcessor(ProcessorSolana)
}

// SetProcessorConfig sets the configuration for a specific processor
func (p *Price) SetProcessorConfig(processor Processor, config map[string]string) {
	if p.Processors == nil {
		p.Processors = make(map[string]map[string]string)
	}
	p.Processors[string(processor)] = config
}

// SetNMIConfig sets the NMI processor configuration
func (p *Price) SetNMIConfig(planID, provider string) {
	config := map[string]string{
		ProcessorKeyPlanID: planID,
	}
	if provider != "" {
		config[ProcessorKeyProvider] = provider
	}
	p.SetProcessorConfig(ProcessorNMI, config)
}

// SetCCBillConfig sets the CCBill processor configuration
func (p *Price) SetCCBillConfig(priceID string) {
	p.SetProcessorConfig(ProcessorCCBill, map[string]string{
		ProcessorKeyPriceID: priceID,
	})
}

// SetSolanaConfig enables the Solana processor
func (p *Price) SetSolanaConfig() {
	p.SetProcessorConfig(ProcessorSolana, map[string]string{
		"enabled": "true",
	})
}
