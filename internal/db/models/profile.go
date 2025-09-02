package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Profile struct {
	bun.BaseModel `bun:"table:profiles,alias:p"`

	ID       uuid.UUID `bun:"id,pk,type:uuid" json:"id" binding:"required"`
	Bio      string    `json:"bio" binding:"required"`
	Username string    `json:"username" binding:"required"`
	UserID   uuid.UUID `bun:",type:uuid" json:"user_id" binding:"required"`

	CreatedAt time.Time `bun:",notnull,type:timestamptz,default:current_timestamp" json:"created_at" binding:"required"`
	UpdatedAt time.Time `bun:",notnull,type:timestamptz,default:current_timestamp" json:"updated_at" binding:"required"`
	DeletedAt time.Time `bun:",soft_delete,type:timestamptz,nullzero" json:"-"`
}
