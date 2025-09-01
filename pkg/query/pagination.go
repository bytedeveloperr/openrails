package query

// PaginatedResponse represents a paginated API response
type PaginatedResponse[T any] struct {
	Items      []T   `json:"items" binding:"required"`
	Page       int   `json:"page" binding:"required"`
	PageSize   int   `json:"page_size" binding:"required"`
	TotalItems int64 `json:"total_items" binding:"required"`
	TotalPages int   `json:"total_pages" binding:"required"`
	HasMore    bool  `json:"has_more" binding:"required"`
}
