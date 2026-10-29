package handler

import (
	"time"

	"srez/internal/entities"
)

type errorResponse struct {
	Error string `json:"error"`
}

type healthResponse struct {
	Status string `json:"status"`
}

type createProductRequest struct {
	BrandID      int64    `json:"brand_id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	PriceKopecks int64    `json:"price_kopecks"`
	Category     string   `json:"category"`
	Materials    []string `json:"materials"`
}

type productResponse struct {
	ID           int64     `json:"id"`
	BrandID      int64     `json:"brand_id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	PriceKopecks int64     `json:"price_kopecks"`
	Category     string    `json:"category"`
	Materials    []string  `json:"materials"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func productDTO(product entities.Product) productResponse {
	materials := product.Materials()
	if materials == nil {
		materials = []string{}
	}
	return productResponse{
		ID: product.ID(), BrandID: product.BrandID(), Name: product.Name(),
		Description: product.Description(), PriceKopecks: product.PriceKopecks(),
		Category: string(product.Category()), Materials: materials, Status: string(product.Status()),
		CreatedAt: product.CreatedAt(), UpdatedAt: product.UpdatedAt(),
	}
}

type brandContactsResponse struct {
	Email       string `json:"email"`
	TelegramURL string `json:"telegram_url"`
	WebsiteURL  string `json:"website_url"`
}

type productDetailResponse struct {
	productResponse
	Colors []productColorResponse `json:"colors"`
}

type productColorResponse struct {
	ID       int64                    `json:"id"`
	Color    string                   `json:"color"`
	Images   []string                 `json:"images"`
	Variants []productVariantResponse `json:"variants"`
}

type productVariantResponse struct {
	ID    int64  `json:"id"`
	Size  string `json:"size"`
	Stock int    `json:"stock"`
	SKU   string `json:"sku"`
}

func productDetailDTO(product entities.Product, publication entities.ProductPublication) productDetailResponse {
	response := productDetailResponse{productResponse: productDTO(product), Colors: make([]productColorResponse, 0, len(publication.Colors))}
	for _, color := range publication.Colors {
		item := productColorResponse{ID: color.ID(), Color: color.Color(), Images: color.Images(), Variants: []productVariantResponse{}}
		for _, variant := range publication.Variants {
			if variant.ProductColorID() == color.ID() {
				item.Variants = append(item.Variants, productVariantResponse{ID: variant.ID(), Size: variant.Size(), Stock: variant.Stock(), SKU: variant.SKU()})
			}
		}
		response.Colors = append(response.Colors, item)
	}
	return response
}

type brandAppearanceResponse struct {
	AccentColor     string `json:"accent_color"`
	BackgroundColor string `json:"background_color"`
	TextColor       string `json:"text_color"`
}

type brandResponse struct {
	ID          int64                   `json:"id"`
	Name        string                  `json:"name"`
	Slug        string                  `json:"slug"`
	Description string                  `json:"description"`
	LogoURL     string                  `json:"logo_url"`
	BannerURL   string                  `json:"banner_url"`
	Contacts    brandContactsResponse   `json:"contacts"`
	Appearance  brandAppearanceResponse `json:"appearance"`
	Status      string                  `json:"status"`
	CreatedAt   time.Time               `json:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

func brandDTO(brand entities.Brand) brandResponse {
	contacts, appearance := brand.Contacts(), brand.Appearance()
	return brandResponse{
		ID: brand.ID(), Name: brand.Name(), Slug: brand.Slug(), Description: brand.Description(),
		LogoURL: brand.LogoURL(), BannerURL: brand.BannerURL(), Status: string(brand.Status()),
		Contacts: brandContactsResponse{
			Email: contacts.Email, TelegramURL: contacts.TelegramURL, WebsiteURL: contacts.WebsiteURL,
		},
		Appearance: brandAppearanceResponse{
			AccentColor: appearance.AccentColor, BackgroundColor: appearance.BackgroundColor, TextColor: appearance.TextColor,
		},
		CreatedAt: brand.CreatedAt(), UpdatedAt: brand.UpdatedAt(),
	}
}
