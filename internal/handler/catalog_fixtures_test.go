package handler

import (
	"srez/internal/devseed"
	"srez/internal/entities"
)

type mockProductEntry struct {
	product     entities.Product
	publication entities.ProductPublication
}

func mockBrands() ([]entities.Brand, error) { return devseed.Brands() }
func mockProductCatalog() ([]mockProductEntry, error) {
	entries, err := devseed.Products()
	if err != nil {
		return nil, err
	}
	result := make([]mockProductEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, mockProductEntry{product: entry.Product, publication: entry.Publication})
	}
	return result, nil
}
