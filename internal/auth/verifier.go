package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	authgin "github.com/doujins-org/authkit/adapters/gin"
	"github.com/doujins-org/authkit/core"
	jwt "github.com/golang-jwt/jwt/v5"

	"github.com/doujins-org/doujins-billing/config"
)

// Claims represents the identity extracted from a verified JWT.
type Claims struct {
	UserID    string
	Email     string
	Username  string
	Roles     []string
	SessionID string
	ExpiresAt time.Time
	Raw       jwt.MapClaims
}

// EmailPtr returns a pointer to the email if present (useful for services expecting *string).
func (c Claims) EmailPtr() *string {
	if c.Email == "" {
		return nil
	}
	e := c.Email
	return &e
}

// Verifier validates bearer tokens against configured issuers/JWKS.
type Verifier interface {
	Verify(ctx context.Context, token string) (*Claims, error)
}

type verifier struct {
	accept core.AcceptConfig
	impl   *authgin.Verifier
}

// NewVerifier builds an authkit-backed verifier using billing auth config.
// Supports multiple issuers to accept tokens from both doujins and hentai0.
func NewVerifier(cfg *config.AuthConfig) (Verifier, error) {
	if cfg == nil {
		return nil, errors.New("auth config is required")
	}

	if len(cfg.Issuers) == 0 {
		return nil, errors.New("at least one auth issuer is required")
	}

	// Build IssuerAccept config for each issuer
	issuerAccepts := make([]core.IssuerAccept, 0, len(cfg.Issuers))
	for _, issuer := range cfg.Issuers {
		// Normalize issuer URL
		issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
		if issuer == "" {
			continue
		}

		// Each issuer uses its own URL as the JWKS base per OIDC spec
		jwksURL := issuer + "/.well-known/jwks.json"

		issCfg := core.IssuerAccept{
			Issuer:  issuer,
			JWKSURL: jwksURL,
		}

		// All issuers should use the same audience (billing-app)
		if cfg.Audience != "" {
			issCfg.Audiences = []string{cfg.Audience}
		}

		issuerAccepts = append(issuerAccepts, issCfg)
	}

	if len(issuerAccepts) == 0 {
		return nil, errors.New("no valid issuers configured")
	}

	accept := core.AcceptConfig{
		Issuers:    issuerAccepts,
		Algorithms: []string{"RS256"},
		Skew:       60 * time.Second,
	}

	impl := authgin.NewVerifier(accept)
	return &verifier{accept: accept, impl: impl}, nil
}

func (v *verifier) Verify(_ context.Context, token string) (*Claims, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("missing_token")
	}

	rawClaims, err := v.impl.Verify(token)
	if err != nil {
		return nil, err
	}

	claims := BuildClaimsFromMap(rawClaims)
	if claims == nil {
		return nil, fmt.Errorf("empty_claims")
	}

	return claims, nil
}

// BuildClaimsFromMap converts a raw JWT map into structured claims.
func BuildClaimsFromMap(raw jwt.MapClaims) *Claims {
	if raw == nil {
		return nil
	}
	claims := Claims{Raw: raw}
	claims.UserID = stringVal(raw["sub"])
	claims.Email = stringVal(raw["email"])
	claims.Username = stringVal(raw["preferred_username"])
	if claims.Username == "" {
		claims.Username = stringVal(raw["username"])
	}
	claims.SessionID = stringVal(raw["session_id"])
	claims.Roles = toStringSlice(raw["roles"])

	switch exp := raw["exp"].(type) {
	case float64:
		claims.ExpiresAt = time.Unix(int64(exp), 0)
	case int64:
		claims.ExpiresAt = time.Unix(exp, 0)
	case json.Number:
		if v, err := exp.Int64(); err == nil {
			claims.ExpiresAt = time.Unix(v, 0)
		}
	}

	return &claims
}

func stringVal(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func toStringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, el := range t {
			if s, ok := el.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// FormatVerifierError normalises verifier error messages for HTTP responses.
func FormatVerifierError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "missing_token"):
		return "missing_token"
	case strings.Contains(msg, "bad_issuer"):
		return "invalid_issuer"
	case strings.Contains(msg, "bad_audience"):
		return "invalid_audience"
	case strings.Contains(msg, "invalid_token"):
		return "invalid_or_expired_token"
	default:
		return msg
	}
}
