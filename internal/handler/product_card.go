package handler

import (
	"strings"

	"srez/internal/usecase"
)

type productCardResponse struct {
	productResponse
	PreviewImageURL string                     `json:"preview_image_url"`
	BrandName       string                     `json:"brand_name"`
	BrandSlug       string                     `json:"brand_slug"`
	IsAvailable     bool                       `json:"is_available"`
	AvailableColors []string                   `json:"available_colors"`
	AvailableSizes  []string                   `json:"available_sizes"`
	Colors          []productCardColorResponse `json:"colors"`
}

type productCardColorResponse struct {
	ID              int64    `json:"id"`
	Color           string   `json:"color"`
	PreviewImageURL string   `json:"preview_image_url"`
	IsAvailable     bool     `json:"is_available"`
	AvailableSizes  []string `json:"available_sizes"`
}

func productCardDTO(card usecase.ProductCard) productCardResponse {
	response := productCardResponse{
		productResponse: productDTO(card.Product), PreviewImageURL: card.PreviewImageURL,
		BrandName: card.BrandName, BrandSlug: card.BrandSlug,
		AvailableColors: []string{}, AvailableSizes: []string{}, Colors: []productCardColorResponse{},
	}
	allSizes := map[string]bool{}
	for _, color := range card.Colors {
		item := productCardColorResponse{ID: color.ID, Color: color.Color, PreviewImageURL: color.PreviewImageURL, AvailableSizes: []string{}}
		colorSizes := map[string]bool{}
		for _, variant := range color.Variants {
			if variant.Stock <= 0 {
				continue
			}
			item.IsAvailable = true
			key := strings.ToLower(strings.TrimSpace(variant.Size))
			if !colorSizes[key] {
				item.AvailableSizes = append(item.AvailableSizes, variant.Size)
				colorSizes[key] = true
			}
			if !allSizes[key] {
				response.AvailableSizes = append(response.AvailableSizes, variant.Size)
				allSizes[key] = true
			}
		}
		if item.IsAvailable {
			response.IsAvailable = true
			response.AvailableColors = append(response.AvailableColors, color.Color)
		}
		response.Colors = append(response.Colors, item)
	}
	return response
}
