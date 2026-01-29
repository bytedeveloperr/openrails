package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/open-rails/openrails/internal/db/models"
	"github.com/open-rails/openrails/internal/services"
)

// -------------------------------- Definition Surface (Host/Admin) --------------------------------

type CreditType struct {
	Name          string
	DisplayName   string
	Unit          string
	DecimalPlaces int
	IsActive      bool
	CreatedAt     time.Time
}

type CreateCreditTypeRequest struct {
	Name          string
	DisplayName   string
	Unit          string
	DecimalPlaces int
}

func (s *Service) CreateCreditType(ctx context.Context, req CreateCreditTypeRequest) (*CreditType, error) {
	if s == nil || s.rt == nil || s.rt.CreditTypeService == nil {
		return nil, fmt.Errorf("billing service: credit type service unavailable")
	}
	req.Name = strings.TrimSpace(req.Name)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Unit = strings.TrimSpace(req.Unit)
	if req.Name == "" {
		return nil, fmt.Errorf("name required")
	}
	if req.DisplayName == "" {
		return nil, fmt.Errorf("display_name required")
	}
	if req.Unit == "" {
		return nil, fmt.Errorf("unit required")
	}
	ct := &models.CreditType{
		Name:          req.Name,
		DisplayName:   req.DisplayName,
		Unit:          req.Unit,
		DecimalPlaces: req.DecimalPlaces,
	}
	if err := s.rt.CreditTypeService.Create(ctx, ct); err != nil {
		return nil, err
	}
	return &CreditType{
		Name:          ct.Name,
		DisplayName:   ct.DisplayName,
		Unit:          ct.Unit,
		DecimalPlaces: ct.DecimalPlaces,
		IsActive:      ct.IsActive,
		CreatedAt:     ct.CreatedAt,
	}, nil
}

type UpdateCreditTypeRequest struct {
	DisplayName *string
	IsActive    *bool
}

func (s *Service) UpdateCreditType(ctx context.Context, name string, req UpdateCreditTypeRequest) (*CreditType, error) {
	if s == nil || s.rt == nil || s.rt.CreditTypeService == nil {
		return nil, fmt.Errorf("billing service: credit type service unavailable")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name required")
	}
	ct, err := s.rt.CreditTypeService.Update(ctx, name, services.CreditTypeUpdateParams{
		DisplayName: req.DisplayName,
		IsActive:    req.IsActive,
	})
	if err != nil {
		return nil, err
	}
	return &CreditType{
		Name:          ct.Name,
		DisplayName:   ct.DisplayName,
		Unit:          ct.Unit,
		DecimalPlaces: ct.DecimalPlaces,
		IsActive:      ct.IsActive,
		CreatedAt:     ct.CreatedAt,
	}, nil
}

func (s *Service) DeactivateCreditType(ctx context.Context, name string) error {
	if s == nil || s.rt == nil || s.rt.CreditTypeService == nil {
		return fmt.Errorf("billing service: credit type service unavailable")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name required")
	}
	return s.rt.CreditTypeService.Deactivate(ctx, name)
}

func (s *Service) ActivateCreditType(ctx context.Context, name string) error {
	if s == nil || s.rt == nil || s.rt.CreditTypeService == nil {
		return fmt.Errorf("billing service: credit type service unavailable")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name required")
	}
	return s.rt.CreditTypeService.Activate(ctx, name)
}

func (s *Service) ListCreditTypes(ctx context.Context, activeOnly bool) ([]CreditType, error) {
	if s == nil || s.rt == nil || s.rt.CreditTypeService == nil {
		return nil, fmt.Errorf("billing service: credit type service unavailable")
	}
	items, err := s.rt.CreditTypeService.List(ctx, activeOnly)
	if err != nil {
		return nil, err
	}
	out := make([]CreditType, 0, len(items))
	for _, ct := range items {
		out = append(out, CreditType{
			Name:          ct.Name,
			DisplayName:   ct.DisplayName,
			Unit:          ct.Unit,
			DecimalPlaces: ct.DecimalPlaces,
			IsActive:      ct.IsActive,
			CreatedAt:     ct.CreatedAt,
		})
	}
	return out, nil
}
