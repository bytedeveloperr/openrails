package services

import (
	"context"
	"fmt"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// PublicSubscriptionService handles public-facing subscription operations
type PublicSubscriptionService struct {
	DB *db.DB
}

// GetActiveProducts retrieves all active products
func (s *PublicSubscriptionService) GetActiveProducts(ctx context.Context) ([]*models.Product, error) {
	var products []*models.Product
	err := s.DB.GetDB().NewSelect().
		Model(&products).
		Where("active = ?", true).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active products: %w", err)
	}
	return products, nil
}

// GetPricesByProductID retrieves prices for a specific product
func (s *PublicSubscriptionService) GetPricesByProductID(ctx context.Context, productID uuid.UUID) ([]*models.Price, error) {
	var prices []*models.Price
	err := s.DB.GetDB().NewSelect().
		Model(&prices).
		Where("product_id = ?", productID).
		Where("active = ?", true).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get prices by product ID: %w", err)
	}
	return prices, nil
}

// PublicProductResponse represents a product with pricing for public display
type PublicProductResponse struct {
	*models.Product
	Prices []*models.Price `json:"prices"`
}

// GetAvailableProducts returns all active products with their prices for public consumption
func (s *PublicSubscriptionService) GetAvailableProducts(ctx context.Context) ([]*PublicProductResponse, error) {
	products, err := s.GetActiveProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get products: %w", err)
	}

	responses := make([]*PublicProductResponse, len(products))
	for i, product := range products {
		responses[i] = &PublicProductResponse{
			Product: product,
		}

		// Get prices for this product
		prices, err := s.GetPricesByProductID(ctx, product.ID)
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
