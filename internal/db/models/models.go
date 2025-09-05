package models

import (
	"github.com/uptrace/bun"
)

// GrantSource represents the source of a role grant
type GrantSource string

const (
	GrantSourceSubscription GrantSource = "subscription"
	GrantSourcePurchase     GrantSource = "purchase"
	GrantSourceAdmin        GrantSource = "admin"
)

// Processor represents payment processor types
type Processor string

const (
	ProcessorMobius Processor = "mobius"
	ProcessorCCBill Processor = "ccbill"
	ProcessorSolana Processor = "solana"
	ProcessorPayPal Processor = "paypal"
)

var ModelRegistry = []any{
    (*Role)(nil),
    (*Product)(nil),
    (*Price)(nil),
    (*Payment)(nil),
    (*Purchase)(nil),
    (*Subscription)(nil),
    (*PaymentMethod)(nil),
    (*SolanaWallet)(nil),
    (*NotificationQueue)(nil),
    (*UserRoleGrant)(nil),
    (*IdempotencyRequest)(nil),
}

func RegisterModels(db *bun.DB) {
	db.RegisterModel(ModelRegistry...)
}
