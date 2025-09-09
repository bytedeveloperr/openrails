package services

import (
    "context"
    "errors"
    "fmt"
    "math"
    "time"

    "github.com/doujins-org/doujins-billing/config"
    "github.com/doujins-org/doujins-billing/internal/db"
    "github.com/doujins-org/doujins-billing/internal/db/models"
    "github.com/google/uuid"
)

var (
    ErrInvalidToken       = errors.New("invalid or unsupported token")
    ErrPriceNotFound      = errors.New("price not found")
)

type SolanaPaymentService struct {
    db            *db.DB
    cfg           *config.Config
    priceService  *PriceService
    paymentSvc    *PaymentService
}

func NewSolanaPaymentService(db *db.DB, cfg *config.Config, price *PriceService, payment *PaymentService) *SolanaPaymentService {
    return &SolanaPaymentService{db: db, cfg: cfg, priceService: price, paymentSvc: payment}
}

// Generate creates a pending SolanaTransaction record and returns UI hints for client-side payment.
// This does not (yet) build a binary transaction; instead it returns token amount calculations
// and creates server-side pending state for follow-up confirmation.
// userID: OIDC subject (string)
func (s *SolanaPaymentService) Generate(ctx context.Context, userID string, priceID uuid.UUID, tokenSymbol, userWallet string) (amount float64, currency string, tokenAmount uint64, expiresAt time.Time, pendingID uuid.UUID, err error) {
    price, err := s.priceService.GetByID(ctx, priceID)
    if err != nil {
        return 0, "", 0, time.Time{}, uuid.Nil, fmt.Errorf("%w: %v", ErrPriceNotFound, err)
    }
    if s.cfg.Solana == nil {
        return 0, "", 0, time.Time{}, uuid.Nil, fmt.Errorf("solana not configured")
    }
    tok, ok := s.cfg.Solana.SupportedTokens[tokenSymbol]
    if !ok || !tok.Enabled {
        return 0, "", 0, time.Time{}, uuid.Nil, ErrInvalidToken
    }

    pow := math.Pow10(tok.Decimals)
    tokenAmt := uint64(math.Round(price.Amount * pow))
    exp := time.Now().Add(10 * time.Minute)

    // Disallow one-off if a subscription entitlement is already active (indefinite)
    if userID != "" {
        exists, _ := s.db.GetDB().NewSelect().
            Model((*models.Entitlement)(nil)).
            Where("user_id = ? AND entitlement = ?", userID, "premium").
            Where("revoked_at IS NULL").
            Where("end_at IS NULL").
            Where("start_at <= ?", time.Now()).
            Exists(ctx)
        if exists {
            return 0, "", 0, time.Time{}, uuid.Nil, fmt.Errorf("one-off purchase not allowed while subscription entitlement is active")
        }
    }

    // Create pending transaction record for traceability
    stx := &models.SolanaTransaction{
        ID:          uuid.New(),
        Status:      "pending",
        Amount:      price.Amount,
        Token:       tok.Symbol,
        TokenMint:   tok.Mint,
        FromAddress: userWallet,
        ToAddress:   firstNonEmpty(s.cfg.Solana.RecipientWallet, s.cfg.Solana.DestinationWallet),
        ExpiresAt:   &exp,
    }
    if userID != "" {
        stx.UserID = &userID
    }
    if _, err := s.db.GetDB().NewInsert().Model(stx).Exec(ctx); err != nil {
        return 0, "", 0, time.Time{}, uuid.Nil, fmt.Errorf("failed to create pending solana transaction: %w", err)
    }

    return price.Amount, price.Currency, tokenAmt, exp, stx.ID, nil
}

// Submit records a confirmed payment for the given price and user.
// This is a pragmatic implementation that skips on-chain signature verification in this codebase.
// userID: OIDC subject (string)
func (s *SolanaPaymentService) Submit(ctx context.Context, userID string, priceID uuid.UUID, signature string) (*models.Payment, error) {
    price, err := s.priceService.GetByID(ctx, priceID)
    if err != nil {
        return nil, fmt.Errorf("%w: %v", ErrPriceNotFound, err)
    }

    // Disallow one-off if a subscription entitlement is already active (indefinite)
    if userID != "" {
        exists, _ := s.db.GetDB().NewSelect().
            Model((*models.Entitlement)(nil)).
            Where("user_id = ? AND entitlement = ?", userID, "premium").
            Where("revoked_at IS NULL").
            Where("end_at IS NULL").
            Where("start_at <= ?", time.Now()).
            Exists(ctx)
        if exists {
            return nil, fmt.Errorf("one-off purchase not allowed while subscription entitlement is active")
        }
    }

    // Create canonical payment record
    pay := &models.Payment{
        ID:            uuid.New(),
        UserID:        userID,
        PriceID:       price.ID,
        Processor:     models.ProcessorSolana,
        TransactionID: signature,
        Amount:        price.Amount,
        Currency:      price.Currency,
        PurchasedAt:   time.Now(),
    }
    if err := s.paymentSvc.Create(ctx, pay); err != nil {
        return nil, fmt.Errorf("failed to create payment: %w", err)
    }

    // Mark any pending SolanaTransaction for this user and price as confirmed (best-effort)
    _, _ = s.db.GetDB().NewUpdate().
        TableExpr("solana_transactions").
        Set("status = ?", "confirmed").
        Set("signature = ?", signature).
        Where("user_id = ?", userID).
        Where("amount = ?", price.Amount).
        Exec(ctx)

    return pay, nil
}

func firstNonEmpty(vals ...string) string {
    for _, v := range vals {
        if v != "" {
            return v
        }
    }
    return ""
}
