package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// usdaFoodSource reads USDA FoodData Central. Values are per 100 g for every
// data type used here. The key travels only in the query string to the
// configured base URL and is never logged.
type usdaFoodSource struct {
	baseURL string
	apiKey  string
	client  *http.Client
	now     func() time.Time
}

// NewUSDAFoodSource returns the USDA adapter. An empty key makes every call
// report ErrSourceUnavailable so search degrades to the catalog.
func NewUSDAFoodSource(baseURL, apiKey string, client *http.Client) FoodSource {
	if client == nil {
		client = newFoodSourceClient()
	}
	return &usdaFoodSource{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, client: client, now: time.Now}
}

const (
	usdaSearchDataTypes  = "Foundation,SR Legacy,Branded"
	usdaBarcodeDataTypes = "Branded"
	// FoodData Central nutrient numbers: energy (kcal), protein, carbohydrate
	// by difference, total fat; 957/958 are the Atwater energy variants some
	// Foundation foods publish instead of 208.
	usdaNutrientEnergy         = "208"
	usdaNutrientEnergyGeneral  = "957"
	usdaNutrientEnergySpecific = "958"
	usdaNutrientProtein        = "203"
	usdaNutrientCarbs          = "205"
	usdaNutrientFat            = "204"
)

type usdaSearchResponse struct {
	Foods []usdaFood `json:"foods"`
}

type usdaFood struct {
	FDCID                    int64              `json:"fdcId"`
	Description              string             `json:"description"`
	DataType                 string             `json:"dataType"`
	BrandOwner               string             `json:"brandOwner"`
	BrandName                string             `json:"brandName"`
	GTINUPC                  string             `json:"gtinUpc"`
	ServingSize              float64            `json:"servingSize"`
	ServingSizeUnit          string             `json:"servingSizeUnit"`
	HouseholdServingFullText string             `json:"householdServingFullText"`
	FoodNutrients            []usdaFoodNutrient `json:"foodNutrients"`
}

type usdaFoodNutrient struct {
	NutrientNumber string  `json:"nutrientNumber"`
	UnitName       string  `json:"unitName"`
	Value          float64 `json:"value"`
}

func (s *usdaFoodSource) Search(ctx context.Context, query string, limit int) ([]model.Food, error) {
	items, err := s.search(ctx, query, usdaSearchDataTypes, limit)
	if err != nil {
		return nil, err
	}
	foods := make([]model.Food, 0, len(items))
	for _, item := range items {
		if food, ok := s.toFood(item); ok {
			foods = append(foods, food)
		}
	}
	return foods, nil
}

func (s *usdaFoodSource) LookupBarcode(ctx context.Context, gtin14 string) (*model.Food, bool, error) {
	// FoodData Central has no barcode route; the UPC is indexed for search, so
	// ask with the form the Branded data set stores and accept only an exact
	// normalised match.
	for _, form := range model.ShortGTINForms(gtin14) {
		items, err := s.search(ctx, form, usdaBarcodeDataTypes, 25)
		if err != nil {
			return nil, false, err
		}
		for _, item := range items {
			stored, err := model.NormalizeGTIN(item.GTINUPC)
			if err != nil || stored != gtin14 {
				continue
			}
			if food, ok := s.toFood(item); ok {
				return &food, true, nil
			}
		}
	}
	return nil, false, nil
}

func (s *usdaFoodSource) search(ctx context.Context, query, dataTypes string, limit int) ([]usdaFood, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("usda key not configured: %w", ErrSourceUnavailable)
	}
	params := url.Values{}
	params.Set("api_key", s.apiKey)
	params.Set("query", query)
	params.Set("dataType", dataTypes)
	params.Set("pageSize", strconv.Itoa(limit))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/fdc/v1/foods/search?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("building usda request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling usda: %w", errors.Join(ErrSourceUnavailable, redactURLError(err)))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, sourceStatusError("usda", resp.StatusCode)
	}
	var body usdaSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding usda response: %w", ErrSourceUnavailable)
	}
	return body.Foods, nil
}

// redactURLError strips the URL (which carries the key) from transport errors.
func redactURLError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Errorf("%s request failed: %w", urlErr.Op, urlErr.Err)
	}
	return err
}

func (s *usdaFoodSource) toFood(item usdaFood) (model.Food, bool) {
	var per model.Per100g
	var hasEnergy, hasProtein bool
	energyFallback := map[string]float64{}
	for _, n := range item.FoodNutrients {
		switch n.NutrientNumber {
		case usdaNutrientEnergy:
			if strings.EqualFold(n.UnitName, "kcal") {
				per.EnergyKcal, hasEnergy = n.Value, true
			}
		case usdaNutrientEnergyGeneral, usdaNutrientEnergySpecific:
			energyFallback[n.NutrientNumber] = n.Value
		case usdaNutrientProtein:
			per.ProteinG, hasProtein = n.Value, true
		case usdaNutrientCarbs:
			per.CarbsG = n.Value
		case usdaNutrientFat:
			per.FatG = n.Value
		}
	}
	if !hasEnergy {
		if v, ok := energyFallback[usdaNutrientEnergyGeneral]; ok {
			per.EnergyKcal, hasEnergy = v, true
		} else if v, ok := energyFallback[usdaNutrientEnergySpecific]; ok {
			per.EnergyKcal, hasEnergy = v, true
		}
	}
	name := strings.TrimSpace(item.Description)
	if !hasEnergy || !hasProtein || !model.PlausiblePer100g(per) || name == "" || item.FDCID == 0 {
		return model.Food{}, false
	}
	if len(name) > 200 {
		name = name[:200]
	}
	food := model.Food{
		ID:        uuid.New(),
		Source:    model.FoodSourceUSDA,
		SourceID:  strconv.FormatInt(item.FDCID, 10),
		DataType:  usdaDataType(item.DataType),
		Name:      name,
		Per100g:   per,
		FetchedAt: s.now(),
	}
	if brand := strings.TrimSpace(firstNonEmpty(item.BrandName, item.BrandOwner)); brand != "" {
		if len(brand) > 120 {
			brand = brand[:120]
		}
		food.Brand = &brand
	}
	if gtin, err := model.NormalizeGTIN(item.GTINUPC); err == nil && item.GTINUPC != "" {
		food.GTIN = gtin
		display := model.DisplayGTIN(gtin)
		food.Barcode = &display
	}
	var servings []model.Serving
	if item.ServingSize > 0 && isGramUnit(item.ServingSizeUnit) {
		label := strings.TrimSpace(item.HouseholdServingFullText)
		if label == "" {
			label = fmt.Sprintf("1 serving (%s g)", trimmedNumber(item.ServingSize))
		} else if !strings.Contains(label, " g") {
			label = fmt.Sprintf("%s (%s g)", label, trimmedNumber(item.ServingSize))
		}
		if len(label) > 60 {
			label = label[:60]
		}
		servings = append(servings, model.Serving{Label: label, Grams: item.ServingSize})
	}
	food.Servings = model.EnsureHundredGramServing(servings)
	return food, true
}

func usdaDataType(dataType string) string {
	switch strings.ToLower(strings.TrimSpace(dataType)) {
	case "foundation":
		return "foundation"
	case "sr legacy":
		return "sr_legacy"
	case "branded":
		return "branded"
	default:
		return ""
	}
}

func isGramUnit(unit string) bool {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "g", "grm", "gram", "grams":
		return true
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func trimmedNumber(v float64) string {
	return strings.TrimSuffix(strconv.FormatFloat(v, 'f', 1, 64), ".0")
}
