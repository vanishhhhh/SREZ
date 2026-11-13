package handler

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"srez/internal/entities"
	"srez/internal/usecase"
)

func parseProductQuery(values url.Values) (usecase.ProductQuery, error) {
	q := usecase.ProductQuery{Page: 1, Limit: usecase.DefaultProductLimit}
	for _, key := range []string{"category", "brand_id", "min_price", "max_price", "size", "color", "sort", "page", "limit"} {
		entries, exists := values[key]
		if !exists {
			continue
		}
		if len(entries) != 1 || strings.TrimSpace(entries[0]) == "" {
			return q, fmt.Errorf("%w: %s", usecase.ErrInvalidProductQuery, key)
		}
		value := entries[0]
		switch key {
		case "category":
			q.Category = entities.ProductCategory(value)
		case "size":
			q.Size = value
		case "color":
			q.Color = value
		case "sort":
			q.Sort = value
		default:
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n < 0 || ((key == "brand_id" || key == "page" || key == "limit") && n == 0) {
				return q, fmt.Errorf("%w: %s", usecase.ErrInvalidProductQuery, key)
			}
			switch key {
			case "brand_id":
				q.BrandID = n
			case "min_price":
				q.MinPrice = &n
			case "max_price":
				q.MaxPrice = &n
			case "page":
				q.Page = n
			case "limit":
				if n > usecase.MaxProductLimit {
					return q, fmt.Errorf("%w: limit", usecase.ErrInvalidProductQuery)
				}
				q.Limit = int(n)
			}
		}
	}
	return usecase.NormalizeProductQuery(q)
}
