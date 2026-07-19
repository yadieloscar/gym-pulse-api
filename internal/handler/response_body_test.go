package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONRejectsTrailingValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"value":1} {"value":2}`))
	rec := httptest.NewRecorder()
	var body map[string]int
	if err := decodeJSON(rec, req, &body); err == nil {
		t.Fatal("trailing JSON value was accepted")
	}
}

func TestDecodeJSONMapsOversizedBody(t *testing.T) {
	body := `{"value":"` + strings.Repeat("x", maxJSONBodyBytes) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	var target map[string]string
	err := decodeJSON(rec, req, &target)
	if err == nil {
		t.Fatal("oversized JSON body was accepted")
	}
	writeDecodeError(rec, err)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
	var response apiError
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Code != "REQUEST_TOO_LARGE" {
		t.Fatalf("code = %q", response.Code)
	}
}
