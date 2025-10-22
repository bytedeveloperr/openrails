package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/integrations/nmi"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

type VaultService struct {
	PaymentMethodService *PaymentMethodService
	SubscriptionService  *SubscriptionService
	NMIClient            *nmi.NMIClient
	DB                   *db.DB
}

type CreateVaultRequest struct {
	PaymentToken string
	CCNumber     string
	CCExp        string
	FirstName    string
	LastName     string
	Address1     string
	City         string
	State        string
	Zip          string
	Country      string
	Phone        string
	Email        string
	Company      string
	Address2     string
}

type UpdateVaultRequest struct {
	CCNumber  *string
	CCExp     *string
	FirstName *string
	LastName  *string
	Address1  *string
	City      *string
	State     *string
	Zip       *string
	Country   *string
	Phone     *string
	Email     *string
	Company   *string
	Address2  *string
}

func NewVaultService(pm *PaymentMethodService, sub *SubscriptionService, mob *nmi.NMIClient, dbx *db.DB) *VaultService {
	return &VaultService{
		PaymentMethodService: pm,
		SubscriptionService:  sub,
		NMIClient:            mob,
		DB:                   dbx,
	}
}

// CreateVault creates a NMI customer vault and stores a local PaymentMethod
func (s *VaultService) CreateVault(ctx context.Context, user *UserIdentity, req *CreateVaultRequest) (*models.PaymentMethod, error) {
	vaultData := nmi.CreateCustomerVaultData{
		PaymentToken: req.PaymentToken,
		CCNumber:     req.CCNumber,
		CCExp:        req.CCExp,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Address1:     req.Address1,
		City:         req.City,
		State:        req.State,
		Zip:          req.Zip,
		Country:      req.Country,
		Phone:        req.Phone,
		Email:        req.Email,
		Company:      req.Company,
		Address2:     req.Address2,
	}

	nmiResponse, err := s.NMIClient.CreateCustomerVault(vaultData)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{"user_id": user.ID}).Error("Failed to create vault in NMI")
		return nil, fmt.Errorf("failed to create payment vault: %w", err)
	}

	uid, perr := uuid.Parse(user.ID)
	if perr != nil {
		return nil, fmt.Errorf("invalid user id: %w", perr)
	}
	pm := &models.PaymentMethod{
		ID:                   uuid.New(),
		UserID:               user.ID,
		Processor:            models.ProcessorNMI,
		VaultID:              nmiResponse.CustomerVaultID,
		InitialTransactionID: "",
		IsActive:             true,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}

	if err := s.PaymentMethodService.Create(ctx, pm); err != nil {
		log.WithError(err).WithFields(log.Fields{"user_id": user.ID, "vault_id": nmiResponse.CustomerVaultID}).Error("Failed to store vault locally")
		// Attempt remote cleanup
		_ = s.NMIClient.DeleteCustomerVault(nmi.DeleteCustomerVaultData{CustomerVaultID: nmiResponse.CustomerVaultID})
		return nil, fmt.Errorf("failed to store vault locally: %w", err)
	}

	log.WithFields(log.Fields{"user_id": user.ID, "vault_id": pm.VaultID}).Info("Successfully created payment vault")
	return pm, nil
}

// UpdateVault updates vault in NMI and updates local record timestamp
func (s *VaultService) UpdateVault(ctx context.Context, pm *models.PaymentMethod, req *UpdateVaultRequest) (*models.PaymentMethod, error) {
	upd := nmi.UpdateCustomerVaultData{CustomerVaultID: pm.VaultID}
	if req.CCNumber != nil {
		upd.CCNumber = *req.CCNumber
	}
	if req.CCExp != nil {
		upd.CCExp = *req.CCExp
	}
	if req.FirstName != nil {
		upd.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		upd.LastName = *req.LastName
	}
	if req.Address1 != nil {
		upd.Address1 = *req.Address1
	}
	if req.City != nil {
		upd.City = *req.City
	}
	if req.State != nil {
		upd.State = *req.State
	}
	if req.Zip != nil {
		upd.Zip = *req.Zip
	}
	if req.Country != nil {
		upd.Country = *req.Country
	}
	if req.Phone != nil {
		upd.Phone = *req.Phone
	}
	if req.Email != nil {
		upd.Email = *req.Email
	}
	if req.Company != nil {
		upd.Company = *req.Company
	}
	if req.Address2 != nil {
		upd.Address2 = *req.Address2
	}

	if err := s.NMIClient.UpdateCustomerVault(upd); err != nil {
		log.WithError(err).WithField("vault_id", pm.VaultID).Error("Failed to update vault in NMI")
		return nil, fmt.Errorf("failed to update payment vault: %w", err)
	}

	pm.UpdatedAt = time.Now()
	if err := s.PaymentMethodService.Update(ctx, pm); err != nil {
		log.WithError(err).WithField("vault_id", pm.VaultID).Error("Failed to update local vault record")
		return nil, fmt.Errorf("failed to update local vault record: %w", err)
	}
	log.WithField("vault_id", pm.VaultID).Info("Successfully updated payment vault")
	return pm, nil
}

// DeleteVault deletes the vault remotely after ensuring no active subscriptions use it; deactivates locally
func (s *VaultService) DeleteVault(ctx context.Context, pm *models.PaymentMethod) error {
	subs, _, err := s.SubscriptionService.GetPaginatedByUserID(ctx, pm.UserID.String(), 1, 1000)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{"vault_id": pm.VaultID, "user_id": pm.UserID}).Error("Failed to check subscriptions for vault")
		return fmt.Errorf("failed to check vault usage: %w", err)
	}

	activeCount := 0
	for _, sub := range subs {
		if sub.Status == models.StatusActive || sub.Status == models.StatusPastDue {
			if sub.PaymentMethodID != nil && *sub.PaymentMethodID == pm.ID {
				activeCount++
			}
		}
	}
	if activeCount > 0 {
		return fmt.Errorf("cannot delete vault: %d active subscription(s) are using this payment method", activeCount)
	}

	if err := s.NMIClient.DeleteCustomerVault(nmi.DeleteCustomerVaultData{CustomerVaultID: pm.VaultID}); err != nil {
		log.WithError(err).WithField("vault_id", pm.VaultID).Error("Failed to delete vault from NMI")
		return fmt.Errorf("failed to delete payment vault: %w", err)
	}

	pm.IsActive = false
	pm.UpdatedAt = time.Now()
	if err := s.PaymentMethodService.Update(ctx, pm); err != nil {
		log.WithError(err).WithField("vault_id", pm.VaultID).Error("Failed to deactivate vault locally")
		return fmt.Errorf("failed to deactivate local vault record: %w", err)
	}

	log.WithField("vault_id", pm.VaultID).Info("Successfully deleted payment vault")
	return nil
}

// ActivateVault sets this vault as active for the user and deactivates others
func (s *VaultService) ActivateVault(ctx context.Context, pm *models.PaymentMethod) (*models.PaymentMethod, error) {
	if !pm.IsActive {
		return nil, errors.New("cannot activate inactive vault")
	}

	if err := s.PaymentMethodService.DeactivateByUserID(ctx, pm.UserID.String()); err != nil {
		log.WithError(err).WithField("user_id", pm.UserID).Error("Failed to deactivate other vaults")
		return nil, fmt.Errorf("failed to deactivate other payment methods: %w", err)
	}
	if err := s.PaymentMethodService.ActivateByID(ctx, pm.ID); err != nil {
		log.WithError(err).WithField("vault_id", pm.ID).Error("Failed to activate vault")
		return nil, fmt.Errorf("failed to activate payment method: %w", err)
	}

	updated, err := s.PaymentMethodService.GetByID(ctx, pm.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh vault: %w", err)
	}

	log.WithFields(log.Fields{"user_id": pm.UserID, "vault_id": pm.ID}).Info("Successfully activated payment vault")
	return updated, nil
}

// GetUserVaults lists vaults for user (optionally including inactive)
func (s *VaultService) GetUserVaults(ctx context.Context, userID string, includeInactive bool) ([]*models.PaymentMethod, error) {
	if includeInactive {
		return s.PaymentMethodService.GetByUserID(ctx, userID)
	}
	return s.PaymentMethodService.GetActiveByUserID(ctx, userID)
}

// GetUserActiveVault returns the active vault for a user
func (s *VaultService) GetUserActiveVault(ctx context.Context, userID string) (*models.PaymentMethod, error) {
	vaults, err := s.PaymentMethodService.GetActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(vaults) == 0 {
		return nil, errors.New("no active payment method found")
	}
	return vaults[0], nil
}
