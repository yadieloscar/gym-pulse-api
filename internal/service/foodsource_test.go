package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

const usdaSearchFixture = `{"foods":[
 {"fdcId":1001,"description":"Greek yogurt, plain, nonfat","dataType":"Branded","brandName":"Kirkland Signature","brandOwner":"Costco","gtinUpc":"096619123452",
  "servingSize":170,"servingSizeUnit":"g","householdServingFullText":"1 container",
  "foodNutrients":[{"nutrientNumber":"208","unitName":"KCAL","value":59},{"nutrientNumber":"203","unitName":"G","value":10.2},{"nutrientNumber":"205","unitName":"G","value":3.6},{"nutrientNumber":"204","unitName":"G","value":0.4}]},
 {"fdcId":1002,"description":"Chicken, broilers or fryers, breast, cooked","dataType":"SR Legacy",
  "foodNutrients":[{"nutrientNumber":"208","unitName":"KCAL","value":165},{"nutrientNumber":"203","unitName":"G","value":31},{"nutrientNumber":"205","unitName":"G","value":0},{"nutrientNumber":"204","unitName":"G","value":3.6}]},
 {"fdcId":1003,"description":"Implausible oil","dataType":"Foundation",
  "foodNutrients":[{"nutrientNumber":"208","unitName":"KCAL","value":950},{"nutrientNumber":"203","unitName":"G","value":0},{"nutrientNumber":"204","unitName":"G","value":100}]},
 {"fdcId":1004,"description":"No protein published","dataType":"Branded","gtinUpc":"096619000000",
  "foodNutrients":[{"nutrientNumber":"208","unitName":"KCAL","value":100}]},
 {"fdcId":1005,"description":"Olive oil","dataType":"Foundation","servingSize":15,"servingSizeUnit":"ml",
  "foodNutrients":[{"nutrientNumber":"957","unitName":"KCAL","value":884},{"nutrientNumber":"203","unitName":"G","value":0},{"nutrientNumber":"205","unitName":"G","value":0},{"nutrientNumber":"204","unitName":"G","value":100}]}
]}`

type recordingServer struct {
	mu       sync.Mutex
	requests []*http.Request
	status   int
	body     string
	delay    time.Duration
}

func (s *recordingServer) handler(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests = append(s.requests, r.Clone(r.Context()))
	s.mu.Unlock()
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	if s.status != 0 && s.status != http.StatusOK {
		w.WriteHeader(s.status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(s.body))
}

func TestUSDASearchMapsFieldsAndFilters(t *testing.T) {
	server := &recordingServer{body: usdaSearchFixture}
	ts := httptest.NewServer(http.HandlerFunc(server.handler))
	defer ts.Close()
	source := NewUSDAFoodSource(ts.URL, "secret-key", nil)
	foods, err := source.Search(context.Background(), "greek yogurt", 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(foods) != 3 {
		t.Fatalf("expected the two plausible items plus the Atwater-energy oil, got %d: %+v", len(foods), foods)
	}
	yogurt := foods[0]
	if yogurt.Source != model.FoodSourceUSDA || yogurt.SourceID != "1001" || yogurt.DataType != "branded" || yogurt.Brand == nil || *yogurt.Brand != "Kirkland Signature" {
		t.Errorf("yogurt mapping: %+v", yogurt)
	}
	if yogurt.Barcode == nil || *yogurt.Barcode != "0096619123452" || yogurt.GTIN != "00096619123452" {
		t.Errorf("barcode normalised: %+v %s", yogurt.Barcode, yogurt.GTIN)
	}
	if yogurt.Per100g != (model.Per100g{EnergyKcal: 59, ProteinG: 10.2, CarbsG: 3.6, FatG: 0.4}) {
		t.Errorf("per 100 g: %+v", yogurt.Per100g)
	}
	if len(yogurt.Servings) != 2 || yogurt.Servings[0] != (model.Serving{Label: "1 container (170 g)", Grams: 170}) || yogurt.Servings[1] != model.HundredGramServing {
		t.Errorf("servings: %+v", yogurt.Servings)
	}
	if foods[1].DataType != "sr_legacy" || foods[1].Barcode != nil || foods[1].Attribution != nil || len(foods[1].Servings) != 1 {
		t.Errorf("generic food: %+v", foods[1])
	}
	if foods[2].Per100g.EnergyKcal != 884 || len(foods[2].Servings) != 1 {
		t.Errorf("Atwater energy fallback and ml serving skipped: %+v", foods[2])
	}
	req := server.requests[0]
	q := req.URL.Query()
	if req.URL.Path != "/fdc/v1/foods/search" || q.Get("api_key") != "secret-key" || q.Get("query") != "greek yogurt" || q.Get("dataType") != usdaSearchDataTypes || q.Get("pageSize") != "25" {
		t.Errorf("request: %s %v", req.URL.Path, q)
	}
}

func TestUSDAUnavailableAndKeyHandling(t *testing.T) {
	noKey := NewUSDAFoodSource("http://127.0.0.1:1", "", nil)
	if _, err := noKey.Search(context.Background(), "x", 5); !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("empty key must be unavailable, got %v", err)
	}
	server := &recordingServer{status: http.StatusTooManyRequests}
	ts := httptest.NewServer(http.HandlerFunc(server.handler))
	defer ts.Close()
	if _, err := NewUSDAFoodSource(ts.URL, "k", nil).Search(context.Background(), "x", 5); !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("429 must be unavailable, got %v", err)
	}
	slow := &recordingServer{body: usdaSearchFixture, delay: 80 * time.Millisecond}
	slowServer := httptest.NewServer(http.HandlerFunc(slow.handler))
	defer slowServer.Close()
	_, err := NewUSDAFoodSource(slowServer.URL, "secret-key", &http.Client{Timeout: 10 * time.Millisecond}).Search(context.Background(), "x", 5)
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("timeout must be unavailable, got %v", err)
	}
	if strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("key leaked into the error: %v", err)
	}
}

func TestUSDALookupBarcodeMatchesExactGTIN(t *testing.T) {
	server := &recordingServer{body: usdaSearchFixture}
	ts := httptest.NewServer(http.HandlerFunc(server.handler))
	defer ts.Close()
	source := NewUSDAFoodSource(ts.URL, "k", nil)
	food, found, err := source.LookupBarcode(context.Background(), "00096619123452")
	if err != nil || !found || food.SourceID != "1001" {
		t.Fatalf("expected the yogurt, got %+v %v", food, err)
	}
	if q := server.requests[0].URL.Query(); q.Get("query") != "0096619123452" || q.Get("dataType") != usdaBarcodeDataTypes {
		t.Errorf("barcode search asks Branded with the 13-digit form first: %v", q)
	}
	missing, found, err := source.LookupBarcode(context.Background(), "04006381333931")
	if err != nil || found || missing != nil {
		t.Fatalf("unmatched gtin must be a miss, got %+v %v", missing, err)
	}
	if len(server.requests) != 1+1 {
		t.Errorf("an EAN-13 with no shorter form is asked once: %d requests", len(server.requests))
	}
}

func offProductFixture(code string) string {
	body, _ := json.Marshal(map[string]any{
		"status": 1, "code": code,
		"product": map[string]any{
			"product_name": "Hazelnut spread", "brands": "Nutella, Ferrero", "serving_size": "15 g", "serving_quantity": "15",
			"nutriments": map[string]any{"energy-kcal_100g": 539, "proteins_100g": "6.3", "carbohydrates_100g": 57.5, "fat_100g": 30.9},
		},
	})
	return string(body)
}

func TestOpenFoodFactsLookup(t *testing.T) {
	var requests []*http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Clone(r.Context()))
		switch r.URL.Path {
		case "/api/v2/product/3017620422003":
			_, _ = w.Write([]byte(offProductFixture("3017620422003")))
		case "/api/v2/product/0096619123452":
			w.WriteHeader(http.StatusNotFound)
		case "/api/v2/product/096619123452":
			_, _ = w.Write([]byte(`{"status":0,"code":"096619123452"}`))
		case "/api/v2/product/4006381333931":
			_, _ = w.Write([]byte(`{"status":1,"code":"4006381333931","product":{"product_name":"No protein","nutriments":{"energy-kcal_100g":10}}}`))
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer ts.Close()
	source := NewOpenFoodFactsSource(ts.URL, "GymPulse/test (test@example.com)", nil)
	food, found, err := source.LookupBarcode(context.Background(), "03017620422003")
	if err != nil || !found {
		t.Fatalf("expected a product, got %+v %v", food, err)
	}
	if food.Source != model.FoodSourceOpenFoodFacts || food.SourceID != "3017620422003" || food.DataType != "off_product" || food.Brand == nil || *food.Brand != "Nutella" {
		t.Errorf("mapping: %+v", food)
	}
	if food.Per100g != (model.Per100g{EnergyKcal: 539, ProteinG: 6.3, CarbsG: 57.5, FatG: 30.9}) || food.Servings[0] != (model.Serving{Label: "15 g", Grams: 15}) {
		t.Errorf("values and serving: %+v %+v", food.Per100g, food.Servings)
	}
	if food.Attribution == nil || food.Attribution.License != "ODbL" || !strings.HasSuffix(food.Attribution.URL, "/product/3017620422003") {
		t.Errorf("attribution: %+v", food.Attribution)
	}
	if requests[0].Header.Get("User-Agent") != "GymPulse/test (test@example.com)" || requests[0].URL.Query().Get("fields") != openFoodFactsFields {
		t.Errorf("request headers: %v %v", requests[0].Header, requests[0].URL.Query())
	}
	if miss, found, err := source.LookupBarcode(context.Background(), "00096619123452"); err != nil || found {
		t.Fatalf("404 then status 0 is a miss: %+v %v", miss, err)
	}
	if len(requests) != 3 {
		t.Errorf("UPC-A asks the 13-digit form then the 12-digit form: %d requests", len(requests))
	}
	if dropped, found, err := source.LookupBarcode(context.Background(), "04006381333931"); err != nil || found {
		t.Fatalf("a product without protein is dropped: %+v %v", dropped, err)
	}
	if _, _, err := source.LookupBarcode(context.Background(), "05000112637922"); !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("503 must be unavailable, got %v", err)
	}
}

func TestOpenFoodFactsBudgetWindow(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"status":0}`))
	}))
	defer ts.Close()
	source := &openFoodFactsSource{baseURL: ts.URL, userAgent: "t", client: ts.Client(), budget: newRequestBudget(2, time.Minute, func() time.Time { return now }), now: func() time.Time { return now }}
	for range 2 {
		if _, _, err := source.LookupBarcode(context.Background(), "04006381333931"); err != nil {
			t.Fatal(err)
		}
	}
	_, _, err := source.LookupBarcode(context.Background(), "04006381333931")
	var spent *BudgetSpentError
	if !errors.As(err, &spent) || spent.RetryAfter != time.Minute {
		t.Fatalf("third call within the window must be refused with the time to the oldest slot, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("the refused call must not reach the source: %d calls", calls)
	}
	now = now.Add(61 * time.Second)
	if _, _, err := source.LookupBarcode(context.Background(), "04006381333931"); err != nil {
		t.Fatalf("window slides: %v", err)
	}
}
