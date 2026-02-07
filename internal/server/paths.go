package server

// StandaloneV1Prefix is the canonical API prefix for standalone mode.
const StandaloneV1Prefix = "/v1"

// EmbeddedV1Prefix is the canonical API prefix for embedded mode handlers.
//
// Embedded hosts typically mount billing under "/billing", so the stable contract becomes
// "/billing/v1/*".
const EmbeddedV1Prefix = "/billing/v1"

// LegacyV1Prefix is the historical API prefix used by older clients.
//
// Note: this is intentionally "/v/1" (not "/v1").
const LegacyV1Prefix = "/v/1"
