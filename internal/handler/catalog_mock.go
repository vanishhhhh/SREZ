package handler

import (
	"fmt"
	"time"

	"srez/internal/entities"
)

func mockCatalogTime() time.Time {
	return time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
}

// Each call builds fixed examples from scratch. There is no mutable catalog
// or persistence, and POST never changes these fixtures.
func mockBrands() ([]entities.Brand, error) {
	now := mockCatalogTime()
	states := []entities.BrandState{
		{
			ID: 1, Name: "Срез Студия", Slug: "srez-studio", Description: "Базовая одежда на каждый день.",
			LogoURL:    "https://example.com/brands/srez-studio/logo.png",
			BannerURL:  "https://example.com/brands/srez-studio/banner.png",
			Contacts:   entities.BrandContacts{Email: "studio@example.com", TelegramURL: "https://t.me/srez_example", WebsiteURL: "https://example.com/studio"},
			Appearance: entities.BrandAppearance{AccentColor: "#D14532", BackgroundColor: "#FFFFFF", TextColor: "#222222"},
			Status:     entities.BrandActive, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: 2, Name: "Линия", Slug: "linea", Description: "Удобные вещи для города.",
			LogoURL:    "https://example.com/brands/linea/logo.png",
			Contacts:   entities.BrandContacts{WebsiteURL: "https://example.com/linea"},
			Appearance: entities.BrandAppearance{AccentColor: "#2455AA", BackgroundColor: "#F5F5F5", TextColor: "#111111"},
			Status:     entities.BrandActive, CreatedAt: now, UpdatedAt: now,
		},
	}
	brands := make([]entities.Brand, 0, len(states))
	for _, state := range states {
		brand, err := entities.RestoreBrand(state)
		if err != nil {
			return nil, err
		}
		brands = append(brands, *brand)
	}
	return brands, nil
}

type mockProductEntry struct {
	product     entities.Product
	publication entities.ProductPublication
}

func mockProducts() ([]entities.Product, error) {
	entries, err := mockProductCatalog()
	if err != nil {
		return nil, err
	}
	products := make([]entities.Product, 0, len(entries))
	for _, entry := range entries {
		products = append(products, entry.product)
	}
	return products, nil
}

func mockProductCatalog() ([]mockProductEntry, error) {
	now := mockCatalogTime()
	states := []entities.ProductState{
		{ID: 1, BrandID: 1, Name: "Футболка База", Description: "Хлопковая футболка свободного кроя.", PriceKopecks: 250000, Category: entities.CategoryTShirt, Materials: []string{"хлопок"}, Status: entities.ProductDraft, CreatedAt: now, UpdatedAt: now},
		{ID: 2, BrandID: 2, Name: "Худи Город", Description: "Худи с капюшоном для прохладной погоды.", PriceKopecks: 650000, Category: entities.CategoryHoodie, Materials: []string{"хлопок", "полиэстер"}, Status: entities.ProductDraft, CreatedAt: now, UpdatedAt: now},
	}
	products := make([]mockProductEntry, 0, len(states))
	for _, state := range states {
		product, err := entities.RestoreProduct(state)
		if err != nil {
			return nil, err
		}
		publication := entities.ProductPublication{}
		for _, colorState := range []entities.ProductColorState{
			{ID: state.ID*10 + 1, ProductID: state.ID, Color: "Чёрный", Images: []string{
				fmt.Sprintf("https://example.com/products/%d/black.png", state.ID),
				fmt.Sprintf("https://example.com/products/%d/black-back.png", state.ID),
			}},
			{ID: state.ID*10 + 2, ProductID: state.ID, Color: "Белый", Images: []string{
				fmt.Sprintf("https://example.com/products/%d/white.png", state.ID),
			}},
		} {
			color, err := entities.RestoreProductColor(colorState)
			if err != nil {
				return nil, err
			}
			publication.Colors = append(publication.Colors, *color)
		}
		for _, variantState := range []entities.ProductVariantState{
			{ID: state.ID*100 + 1, ProductColorID: state.ID*10 + 1, Size: "M", Stock: 10, SKU: fmt.Sprintf("SREZ-%d-BLK-M", state.ID)},
			{ID: state.ID*100 + 2, ProductColorID: state.ID*10 + 1, Size: "L", Stock: 0, SKU: fmt.Sprintf("SREZ-%d-BLK-L", state.ID)},
			{ID: state.ID*100 + 3, ProductColorID: state.ID*10 + 2, Size: "One Size", Stock: 4, SKU: fmt.Sprintf("SREZ-%d-WHT-OS", state.ID)},
		} {
			variant, err := entities.RestoreProductVariant(variantState)
			if err != nil {
				return nil, err
			}
			publication.Variants = append(publication.Variants, *variant)
		}
		if err := product.Publish(publication, now); err != nil {
			return nil, err
		}
		products = append(products, mockProductEntry{product: *product, publication: publication})
	}
	return products, nil
}
