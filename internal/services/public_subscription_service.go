package services

import (
	"context"
	"fmt"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/db/repo"
	log "github.com/sirupsen/logrus"
)

// PublicSubscriptionService handles public-facing subscription operations
type PublicSubscriptionService struct {
	ProductRepo *repo.ProductRepo
	PriceRepo   *repo.PriceRepo
}

// PublicProductResponse represents a product with pricing for public display
type PublicProductResponse struct {
	*models.Product
	Prices []*models.Price `json:"prices"`
}

// GetAvailableProducts returns all active products with their prices for public consumption
func (s *PublicSubscriptionService) GetAvailableProducts(ctx context.Context) ([]*PublicProductResponse, error) {
	products, err := s.ProductRepo.GetActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get products: %w", err)
	}

	responses := make([]*PublicProductResponse, len(products))
	for i, product := range products {
		responses[i] = &PublicProductResponse{
			Product: product,
		}

		// Get prices for this product
		prices, err := s.PriceRepo.GetByProductID(ctx, product.ID)
		if err != nil {
			log.WithFields(log.Fields{
				"product_id": product.ID,
				"error":      err.Error(),
			}).Warn("Failed to load prices for product in public API")
			prices = []*models.Price{}
		}

		responses[i].Prices = prices
	}

	return responses, nil
}

// GetSubscriptionPageData returns data needed for the subscription page
func (s *PublicSubscriptionService) GetSubscriptionPageData(ctx context.Context) (map[string]any, error) {
	products, err := s.GetAvailableProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get products: %w", err)
	}

	// Build subscription page data
	data := map[string]any{
		"products": products,
		"features": map[string]any{
			"premium": []string{
				"Access to premium content",
				"HD quality downloads",
				"Priority support",
				"No advertisements",
			},
		},
		"currency": "USD", // Currently USD only, future enhancement will support multiple currencies
	}

	return data, nil
}
