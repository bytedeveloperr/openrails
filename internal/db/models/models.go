package models

import (
	"github.com/uptrace/bun"
)

// (Removed) GrantSource: use EntitlementSourceType instead (admin, grace, one_off, subscription)

// Processor represents payment processor types
type Processor string

const (
	ProcessorMobius Processor = "mobius"
	ProcessorCCBill Processor = "ccbill"
	ProcessorSolana Processor = "solana"
	ProcessorPayPal Processor = "paypal"
)

var ModelRegistry = []any{
    (*Product)(nil),
    (*Price)(nil),
    (*Payment)(nil),
    (*Purchase)(nil),
    (*Subscription)(nil),
    (*PaymentMethod)(nil),
    (*SolanaWallet)(nil),
    (*NotificationQueue)(nil),
    (*Entitlement)(nil),
    (*IdempotencyRequest)(nil),
}

func RegisterModels(db *bun.DB) {
	db.RegisterModel(ModelRegistry...)
}
