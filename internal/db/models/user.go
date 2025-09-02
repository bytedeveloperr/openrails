package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// User represents the complete user view combining auth.users and profiles
// This is the primary user type used throughout the application
// Combines: auth.users (authentication) + profiles (username, bio) + roles
//
// NOTE: When loaded via Relation("User"), only auth.users fields will be populated.
// Profile fields (Username, Bio) and Roles need to be fetched separately or use GetFullUser methods.
type User struct {
	bun.BaseModel `bun:"table:auth.users,alias:u"`

	// Core identity fields from auth.users
	ID               uuid.UUID  `bun:",pk,type:uuid" json:"id"`
	Email            *string    `bun:"email" json:"email,omitempty"`
	EmailConfirmedAt *time.Time `bun:"email_confirmed_at" json:"email_confirmed_at,omitempty"`
	LastSignInAt     *time.Time `bun:"last_sign_in_at" json:"last_sign_in_at,omitempty"`
	CreatedAt        time.Time  `bun:"created_at" json:"created_at"`
	UpdatedAt        time.Time  `bun:"updated_at" json:"updated_at"`

	// Profile fields from profiles table (requires JOIN - not loaded via Relation)
	Username  string    `bun:"-" json:"username"`             // Must be fetched with JOIN
	Bio       string    `bun:"-" json:"bio,omitempty"`        // Must be fetched with JOIN
	ProfileID uuid.UUID `bun:"-" json:"profile_id,omitempty"` // Must be fetched with JOIN

	// Role information from user_role_grants (requires separate query)
	Roles []string `bun:"-" json:"roles,omitempty"` // Populated from role grants

	// Fields we explicitly exclude from general use:
	// - EncryptedPassword: Only needed for authentication
	// - Tokens: Only needed for password reset flows
	// - Raw metadata: Implementation details
	// - AvatarURL: Constructed in service layer with full S3 config
}

// ===== SELECTIVE USER DATA TYPES =====
// These types provide context-appropriate user data for different scenarios

// UserAuthor represents author information for public display contexts
// Maps to 'profiles' table to get current username/name (avoiding denormalization)
// Used for: BlogPosts, Comments (for registered users) where author identification is needed
// JOIN logic: blog_posts.author_id = profiles.user_id (both reference auth.users.id)
type UserAuthor struct {
	bun.BaseModel `bun:"table:profiles,alias:p"`
	UserID        uuid.UUID `bun:"user_id,pk,type:uuid" json:"user_id"` // profiles.user_id (references auth.users.id)
	Username      string    `bun:"username" json:"username"`            // Current username from profiles table
	Bio           string    `bun:"bio" json:"bio"`                      // Bio from profiles table
	// Note: Avatar URL should be computed by service layer from asset relationships
}

// UserForAdmin represents full user information for administrative contexts
// Maps to 'auth.users' table (GoTrue's user table) for complete admin user data
// Used for: Admin operations, user management, detailed reporting, reactions
type UserForAdmin struct {
	bun.BaseModel    `bun:"table:auth.users,alias:u"`
	ID               uuid.UUID      `bun:",pk,type:uuid" json:"id"` // auth.users.id (primary key, referenced by other tables)
	Email            string         `json:"email"`
	Phone            string         `json:"phone"`
	CreatedAt        time.Time      `bun:",notnull,type:timestamptz,default:current_timestamp" json:"created_at"`
	UpdatedAt        time.Time      `bun:",notnull,type:timestamptz,default:current_timestamp" json:"updated_at"`
	EmailConfirmedAt *time.Time     `json:"email_confirmed_at,omitempty"`
	PhoneConfirmedAt *time.Time     `json:"phone_confirmed_at,omitempty"`
	LastSignInAt     *time.Time     `json:"last_sign_in_at,omitempty"`
	AppMetadata      map[string]any `bun:",type:jsonb" json:"app_metadata,omitempty"`
	UserMetadata     map[string]any `bun:",type:jsonb" json:"user_metadata,omitempty"`
}

// UserGoTrue represents the raw GoTrue user data from auth.users table
// This is primarily used for authentication and GoTrue compatibility
// For application use, prefer the User view which includes profile data
type UserGoTrue struct {
	bun.BaseModel `bun:"table:auth.users,alias:u"`
	ID            uuid.UUID `bun:",pk,type:uuid" json:"id"` // auth.users.id (primary key, referenced by other tables)
	Email         *string   `json:"email"`
	Role          string    `json:"role"`

	EncryptedPassword *string    `json:"-" bun:"encrypted_password"`
	InvitedAt         *time.Time `json:"invited_at,omitempty" bun:"invited_at"`

	ConfirmationToken  string     `json:"-" bun:"confirmation_token"`
	ConfirmationSentAt *time.Time `json:"confirmation_sent_at,omitempty" bun:"confirmation_sent_at"`

	ConfirmedAt *time.Time `json:"confirmed_at,omitempty" bun:"confirmed_at"`

	RecoveryToken  string     `json:"-" bun:"recovery_token"`
	RecoverySentAt *time.Time `json:"recovery_sent_at,omitempty" bun:"recovery_sent_at"`

	EmailChangeToken  string     `json:"-" bun:"email_change_token"`
	EmailChange       string     `json:"new_email,omitempty" bun:"email_change"`
	EmailChangeSentAt *time.Time `json:"email_change_sent_at,omitempty" bun:"email_change_sent_at"`

	LastSignInAt *time.Time `json:"last_sign_in_at,omitempty" bun:"last_sign_in_at"`

	AppMetaData  json.RawMessage `json:"app_metadata" bun:"raw_app_meta_data"`
	UserMetaData json.RawMessage `json:"user_metadata" bun:"raw_user_meta_data"`

	CreatedAt time.Time `json:"created_at" bun:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bun:"updated_at"`

	ISSuperAdmin bool `json:"is_super_admin" bun:"is_super_admin"`

	DONTUSEINSTANCEID uuid.UUID `json:"-" bun:"instance_id"`
}
