package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// openFoodFactsSource reads Open Food Facts products by barcode only. Its
// published limit is 15 product reads per minute per IP, and the API is one
// IP, so a global budget of 12 protects the whole deployment. Text search is
// never used (10 per minute, shared by every athlete).
type openFoodFactsSource struct {
	baseURL   string
	userAgent string
	client    *http.Client
	budget    *requestBudget
	now       func() time.Time
}

const (
	openFoodFactsBudgetPerMinute = 12
	openFoodFactsFields          = "code,product_name,brands,serving_size,serving_quantity,nutriments"
)

// NewOpenFoodFactsSource returns the Open Food Facts adapter.
func NewOpenFoodFactsSource(baseURL, userAgent string, client *http.Client) FoodSource {
	if client == nil {
		client = newFoodSourceClient()
	}
	return &openFoodFactsSource{
		baseURL:   strings.TrimRight(baseURL, "/"),
		userAgent: userAgent,
		client:    client,
		budget:    newRequestBudget(openFoodFactsBudgetPerMinute, time.Minute, time.Now),
		now:       time.Now,
	}
}

// Search is not offered by this source; see the type comment.
func (s *openFoodFactsSource) Search(context.Context, string, int) ([]model.Food, error) {
	return []model.Food{}, nil
}

type offProductResponse struct {
	Status  int        `json:"status"`
	Code    string     `json:"code"`
	Product offProduct `json:"product"`
}

type offProduct struct {
	ProductName     string          `json:"product_name"`
	Brands          string          `json:"brands"`
	ServingSize     string          `json:"serving_size"`
	ServingQuantity json.RawMessage `json:"serving_quantity"`
	Nutriments      map[string]any  `json:"nutriments"`
}

func (s *openFoodFactsSource) LookupBarcode(ctx context.Context, gtin14 string) (*model.Food, bool, error) {
	for _, code := range model.ShortGTINForms(gtin14) {
		if err := s.budget.take(); err != nil {
			return nil, false, err
		}
		food, found, err := s.fetchProduct(ctx, code, gtin14)
		if err != nil {
			return nil, false, err
		}
		if found {
			return food, true, nil
		}
	}
	return nil, false, nil
}

// fetchProduct asks for one code form; found is false on 404, status 0, or
// an implausible product.
func (s *openFoodFactsSource) fetchProduct(ctx context.Context, code, gtin14 string) (*model.Food, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/api/v2/product/"+code+"?fields="+openFoodFactsFields, nil)
	if err != nil {
		return nil, false, fmt.Errorf("building open food facts request: %w", err)
	}
	req.Header.Set("User-Agent", s.userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("calling open food facts: %w", ErrSourceUnavailable)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, false, nil
	case resp.StatusCode != http.StatusOK:
		return nil, false, sourceStatusError("open food facts", resp.StatusCode)
	}
	var body offProductResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, false, fmt.Errorf("decoding open food facts response: %w", ErrSourceUnavailable)
	}
	if body.Status != 1 {
		return nil, false, nil
	}
	food, ok := s.toFood(body, gtin14)
	if !ok {
		return nil, false, nil
	}
	return &food, true, nil
}

func (s *openFoodFactsSource) toFood(body offProductResponse, gtin14 string) (model.Food, bool) {
	n := body.Product.Nutriments
	energy, hasEnergy := offNumber(n["energy-kcal_100g"])
	protein, hasProtein := offNumber(n["proteins_100g"])
	carbs, _ := offNumber(n["carbohydrates_100g"])
	fat, _ := offNumber(n["fat_100g"])
	per := model.Per100g{EnergyKcal: energy, ProteinG: protein, CarbsG: carbs, FatG: fat}
	name := strings.TrimSpace(body.Product.ProductName)
	code := strings.TrimSpace(body.Code)
	if !hasEnergy || !hasProtein || !model.PlausiblePer100g(per) || name == "" || code == "" {
		return model.Food{}, false
	}
	if len(name) > 200 {
		name = name[:200]
	}
	display := model.DisplayGTIN(gtin14)
	food := model.Food{
		ID:          uuid.New(),
		Source:      model.FoodSourceOpenFoodFacts,
		SourceID:    code,
		DataType:    "off_product",
		GTIN:        gtin14,
		Barcode:     &display,
		Name:        name,
		Per100g:     per,
		Attribution: model.OpenFoodFactsAttribution(code),
		FetchedAt:   s.now(),
	}
	if brand := strings.TrimSpace(strings.Split(body.Product.Brands, ",")[0]); brand != "" {
		if len(brand) > 120 {
			brand = brand[:120]
		}
		food.Brand = &brand
	}
	var servings []model.Serving
	if grams, ok := offNumber(rawToAny(body.Product.ServingQuantity)); ok && grams > 0 {
		label := strings.TrimSpace(body.Product.ServingSize)
		if label == "" {
			label = fmt.Sprintf("1 serving (%s g)", trimmedNumber(grams))
		}
		if len(label) > 60 {
			label = label[:60]
		}
		servings = append(servings, model.Serving{Label: label, Grams: grams})
	}
	food.Servings = model.EnsureHundredGramServing(servings)
	return food, true
}

func rawToAny(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

// offNumber reads a nutriment that OFF may publish as a number or a string.
func offNumber(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	default:
		return 0, false
	}
}
