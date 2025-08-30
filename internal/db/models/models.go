package models

import (
	"github.com/uptrace/bun"
)

var ModelRegistry = []any{
	(*Role)(nil),
	(*Product)(nil),
	(*Price)(nil),
	(*Payment)(nil),
	(*Subscription)(nil),
	(*PaymentMethod)(nil),
	(*NotificationQueue)(nil),
	(*UserRoleGrant)(nil),
	(*IdempotencyRequest)(nil),
}

func RegisterModels(db *bun.DB) {
	db.RegisterModel(ModelRegistry...)
}
