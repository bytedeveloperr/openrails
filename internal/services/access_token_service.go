package services

import (
	"context"
	"fmt"
	"time"

	authkitjwt "github.com/doujins-org/authkit/jwt"
	"github.com/doujins-org/doujins-billing/config"
)

// SignedAccessToken represents a signed JWT plus metadata needed by clients.
type SignedAccessToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	KeyID     string    `json:"kid"`
}

// AccessTokenService signs entitlement/access grants into short-lived JWTs.
type AccessTokenService struct {
	signer   authkitjwt.Signer
	ttl      time.Duration
	issuer   string
	audience string
}

func NewAccessTokenService(cfg *config.SigningConfig) (*AccessTokenService, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, fmt.Errorf("access signing configuration is missing or disabled")
	}

	ttl := 5 * time.Minute
	if cfg.TTL != "" {
		parsed, err := time.ParseDuration(cfg.TTL)
		if err != nil {
			return nil, fmt.Errorf("invalid access signing ttl: %w", err)
		}
		ttl = parsed
	}

	pemBytes := []byte(cfg.PrivateKeyPEM)
	if len(pemBytes) == 0 {
		return nil, fmt.Errorf("access signing private key pem is empty")
	}

	signer, err := authkitjwt.NewRSASignerFromPEM(cfg.KeyID, pemBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to load access signing key: %w", err)
	}

	return &AccessTokenService{
		signer:   signer,
		ttl:      ttl,
		issuer:   cfg.Issuer,
		audience: cfg.Audience,
	}, nil
}

// SignAccessToken emits a signed JWT expressing the provided access grants.
func (s *AccessTokenService) SignAccessToken(ctx context.Context, userID string, grants []*UserAccessGrant) (*SignedAccessToken, error) {
	if s == nil {
		return nil, fmt.Errorf("access token service is not configured")
	}
	if len(grants) == 0 {
		return nil, fmt.Errorf("no access grants available to sign")
	}

	now := time.Now().UTC()
	exp := now.Add(s.ttl)

	claimGrants := make([]map[string]any, 0, len(grants))
	for _, g := range grants {
		if g == nil {
			continue
		}
		claim := map[string]any{
			"kind":        g.Kind,
			"entitlement": g.Entitlement,
			"start_at":    g.StartAt.UTC().Format(time.RFC3339),
		}
		if g.EndAt != nil {
			claim["end_at"] = g.EndAt.UTC().Format(time.RFC3339)
		}
		if g.SourceType != nil {
			claim["source_type"] = *g.SourceType
		}
		if g.SourceID != nil {
			claim["source_id"] = g.SourceID.String()
		}
		if g.SubscriptionID != nil {
			claim["subscription_id"] = g.SubscriptionID.String()
		}
		if g.Processor != "" {
			claim["processor"] = g.Processor
		}
		if g.ProcessorSubscriptionID != nil {
			claim["processor_subscription_id"] = *g.ProcessorSubscriptionID
		}
		claimGrants = append(claimGrants, claim)
	}

	claims := map[string]any{
		"sub":    userID,
		"iat":    now.Unix(),
		"exp":    exp.Unix(),
		"grants": claimGrants,
	}
	if s.issuer != "" {
		claims["iss"] = s.issuer
	}
	if s.audience != "" {
		claims["aud"] = s.audience
	}

	token, err := s.signer.Sign(ctx, claims)
	if err != nil {
		return nil, fmt.Errorf("failed to sign access token: %w", err)
	}

	return &SignedAccessToken{Token: token, ExpiresAt: exp, KeyID: s.signer.KID()}, nil
}
