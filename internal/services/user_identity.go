package services

import ()

// UserIdentity represents the minimal user information needed by billing
type UserIdentity struct {
    ID       string
    Email    *string
    Username string
    Roles    []string
}

// No user directory lookups: Zitadel is the source of truth via JWT claims.
