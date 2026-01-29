package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-rails/openrails/internal/db/models"
	"github.com/open-rails/openrails/internal/db/repo"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"
)

type GrantSubscriptionCreditsParams struct {
	SubscriptionID uuid.UUID
	PeriodEnd      time.Time
	Cadence        models.CreditGrantCadence // once|per_renewal
	Source         string                    // for deposit transaction (e.g., "subscription_initial", "subscription_renewal")
}

func validateCreditGrantSpec(creditTypeName string, spec models.CreditGrantSpec) error {
	if strings.TrimSpace(creditTypeName) == "" {
		return fmt.Errorf("credit type name is empty")
	}
	if spec.Amount <= 0 {
		return fmt.Errorf("invalid credits_spec: %s amount must be > 0", creditTypeName)
	}
	if spec.ExpiresDays != nil && *spec.ExpiresDays <= 0 {
		return fmt.Errorf("invalid credits_spec: %s expires_days must be > 0", creditTypeName)
	}
	cadence := spec.Cadence
	if cadence == "" {
		cadence = models.CreditGrantCadenceOnce
	}
	if cadence != models.CreditGrantCadenceOnce && cadence != models.CreditGrantCadencePerRenewal {
		return fmt.Errorf("invalid credits_spec: %s cadence must be 'once' or 'per_renewal'", creditTypeName)
	}
	return nil
}

// GrantSubscriptionCredits grants credits defined in product.credits_spec for a subscription event.
// It is idempotent per (subscription_id, credit_type_id, period_end).
func (s *CreditsService) GrantSubscriptionCredits(ctx context.Context, params GrantSubscriptionCreditsParams) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("credits service not initialized")
	}
	if params.SubscriptionID == uuid.Nil {
		return fmt.Errorf("subscription_id required")
	}
	if params.PeriodEnd.IsZero() {
		return fmt.Errorf("period_end required")
	}
	if strings.TrimSpace(string(params.Cadence)) == "" {
		return fmt.Errorf("cadence required")
	}
	if strings.TrimSpace(params.Source) == "" {
		return fmt.Errorf("source required")
	}

	tx, err := s.db.GetDB().(*bun.DB).BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := s.now()

	sub := new(models.Subscription)
	if err := tx.NewSelect().
		Model(sub).
		Where("sub.id = ?", params.SubscriptionID).
		Limit(1).
		Scan(ctx); err != nil {
		return err
	}

	prod := new(models.Product)
	if err := tx.NewSelect().
		Model(prod).
		Where("prod.id = ?", sub.ProductID).
		Limit(1).
		Scan(ctx); err != nil {
		return err
	}

	if len(prod.CreditsSpec) == 0 {
		if err := tx.Commit(); err != nil {
			return err
		}
		return nil
	}

	grantRepo := repo.NewSubscriptionCreditGrantRepo(s.db)

	for creditTypeName, spec := range prod.CreditsSpec {
		creditTypeName = strings.TrimSpace(creditTypeName)
		if err := validateCreditGrantSpec(creditTypeName, spec); err != nil {
			return err
		}

		cadence := spec.Cadence
		if cadence == "" {
			cadence = models.CreditGrantCadenceOnce
		}
		if cadence != params.Cadence {
			continue
		}

		ct, err := s.getCreditTypeByNameTx(ctx, tx, creditTypeName)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unknown credit type: %s", creditTypeName)
			}
			return err
		}
		if !ct.IsActive {
			return ErrCreditTypeInactive
		}

		grant := &models.SubscriptionCreditGrant{
			ID:             uuid.New(),
			SubscriptionID: sub.ID,
			CreditTypeID:   ct.ID,
			PeriodEnd:      params.PeriodEnd.UTC(),
			CreatedAt:      now,
		}

		inserted, err := grantRepo.InsertIfNotExists(ctx, tx, grant)
		if err != nil {
			return err
		}
		if !inserted {
			log.WithContext(ctx).WithFields(log.Fields{
				"subscription_id": sub.ID,
				"period_end":      params.PeriodEnd.UTC(),
				"credit_type":     creditTypeName,
				"cadence":         cadence,
			}).Info("subscription credit grant skipped (already granted)")
			continue
		}

		var expiresAt *time.Time
		if spec.ExpiresDays != nil && *spec.ExpiresDays > 0 {
			t := now.Add(time.Duration(*spec.ExpiresDays) * 24 * time.Hour)
			expiresAt = &t
		}

		if _, err := s.depositTx(ctx, tx, ct, CreditDepositParams{
			UserID:     sub.UserID,
			CreditType: creditTypeName,
			Amount:     spec.Amount,
			Source:     params.Source,
			SourceID:   &grant.ID,
			ExpiresAt:  expiresAt,
		}); err != nil {
			return err
		}

		log.WithContext(ctx).WithFields(log.Fields{
			"subscription_id": sub.ID,
			"period_end":      params.PeriodEnd.UTC(),
			"credit_type":     creditTypeName,
			"amount":          spec.Amount,
			"expires_days":    spec.ExpiresDays,
			"cadence":         cadence,
			"grant_id":        grant.ID,
		}).Info("subscription credit grant applied")
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}
