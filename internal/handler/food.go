package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// FoodHandler serves food search, barcode lookup, recents, private foods, and
// food entries under /api/v1/nutrition. See docs/CONTRACTS.md "Food logging".
type FoodHandler struct{ svc service.FoodService }

func NewFoodHandler(svc service.FoodService) *FoodHandler {
	return &FoodHandler{svc: svc}
}

// Search godoc
// @Summary     Search foods by text
// @Description Private foods first, then USDA FoodData Central; catalog only (incomplete=true) when USDA cannot be asked.
// @Tags        nutrition
// @Param       q query string true "2–100 characters"
// @Param       limit query int false "1–25, default 25"
// @Success     200 {object} model.FoodSearchResult
// @Failure     422 {object} map[string]string
// @Failure     429 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/foods/search [get]
func (h *FoodHandler) Search(w http.ResponseWriter, r *http.Request) {
	limit := 25
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "limit must be a whole number", "VALIDATION_ERROR", map[string]string{"field": "limit"})
			return
		}
		limit = parsed
	}
	result, err := h.svc.Search(r.Context(), middleware.MustGetUserID(r.Context()), r.URL.Query().Get("q"), limit)
	respond(w, result, err, http.StatusOK)
}

// LookupBarcode godoc
// @Summary     Look up one product by retail barcode
// @Description Private food → catalog → USDA → Open Food Facts. 404 NOT_FOUND carries details.retry_after_seconds when the fallback budget was spent.
// @Tags        nutrition
// @Param       code path string true "8, 12, 13, or 14 digits"
// @Success     200 {object} model.Food
// @Failure     404 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Failure     429 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/foods/barcode/{code} [get]
func (h *FoodHandler) LookupBarcode(w http.ResponseWriter, r *http.Request) {
	food, err := h.svc.LookupBarcode(r.Context(), middleware.MustGetUserID(r.Context()), chi.URLParam(r, "code"))
	respond(w, food, err, http.StatusOK)
}

// ListRecent godoc
// @Summary     Foods logged in the last 30 days, most often and most recently first
// @Tags        nutrition
// @Success     200 {object} model.RecentFoods
// @Security    BearerAuth
// @Router      /api/v1/nutrition/foods/recent [get]
func (h *FoodHandler) ListRecent(w http.ResponseWriter, r *http.Request) {
	recent, err := h.svc.ListRecent(r.Context(), middleware.MustGetUserID(r.Context()))
	respond(w, recent, err, http.StatusOK)
}

// CreateUserFood godoc
// @Summary     Create a private food from a label
// @Tags        nutrition
// @Param       Idempotency-Key header string true "Equal to operation_key"
// @Param       body body model.SaveUserFoodRequest true "Label values per serving"
// @Success     201 {object} model.Food
// @Failure     409 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/foods [post]
func (h *FoodHandler) CreateUserFood(w http.ResponseWriter, r *http.Request) {
	var req model.SaveUserFoodRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	food, _, err := h.svc.CreateUserFood(r.Context(), middleware.MustGetUserID(r.Context()), req)
	respond(w, food, err, http.StatusCreated)
}

// UpdateUserFood godoc
// @Summary     Change a private food; existing entries keep their snapshot
// @Tags        nutrition
// @Param       id path string true "Food id"
// @Param       Idempotency-Key header string true "Equal to operation_key"
// @Param       body body model.SaveUserFoodRequest true "Label values with expected_revision"
// @Success     200 {object} model.Food
// @Failure     404 {object} map[string]string
// @Failure     409 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/foods/{id} [put]
func (h *FoodHandler) UpdateUserFood(w http.ResponseWriter, r *http.Request) {
	id, ok := foodIDParam(w, r, "food not found")
	if !ok {
		return
	}
	var req model.SaveUserFoodRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	food, _, err := h.svc.UpdateUserFood(r.Context(), middleware.MustGetUserID(r.Context()), id, req)
	respond(w, food, err, http.StatusOK)
}

// ArchiveUserFood godoc
// @Summary     Archive a private food
// @Tags        nutrition
// @Param       id path string true "Food id"
// @Param       expected_revision query int true "Current revision"
// @Param       Idempotency-Key header string true "Operation key"
// @Success     204
// @Failure     404 {object} map[string]string
// @Failure     409 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/foods/{id} [delete]
func (h *FoodHandler) ArchiveUserFood(w http.ResponseWriter, r *http.Request) {
	id, ok := foodIDParam(w, r, "food not found")
	if !ok {
		return
	}
	revision, key, ok := deleteParams(w, r)
	if !ok {
		return
	}
	if _, err := h.svc.ArchiveUserFood(r.Context(), middleware.MustGetUserID(r.Context()), id, revision, key); err != nil {
		handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListDayEntries godoc
// @Summary     A day's totals and its food entries, oldest first
// @Tags        nutrition
// @Param       date path string true "Local calendar date YYYY-MM-DD"
// @Success     200 {object} model.DayEntries
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/days/{date}/entries [get]
func (h *FoodHandler) ListDayEntries(w http.ResponseWriter, r *http.Request) {
	entries, err := h.svc.ListDayEntries(r.Context(), middleware.MustGetUserID(r.Context()), chi.URLParam(r, "date"))
	respond(w, entries, err, http.StatusOK)
}

// CreateEntry godoc
// @Summary     Log a food or a quick add; the day's totals become the sum of its entries
// @Description Typed totals already on the day are converted into one quick_add entry (converted).
// @Tags        nutrition
// @Param       date path string true "Local calendar date YYYY-MM-DD"
// @Param       Idempotency-Key header string true "Equal to operation_key"
// @Param       body body model.CreateFoodEntryRequest true "food+serving+amount, or quick_add"
// @Success     201 {object} model.FoodEntryCreated
// @Failure     404 {object} map[string]string
// @Failure     409 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/days/{date}/entries [post]
func (h *FoodHandler) CreateEntry(w http.ResponseWriter, r *http.Request) {
	var req model.CreateFoodEntryRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	created, _, err := h.svc.CreateEntry(r.Context(), middleware.MustGetUserID(r.Context()), chi.URLParam(r, "date"), req)
	respond(w, created, err, http.StatusCreated)
}

// UpdateEntry godoc
// @Summary     Change an entry's serving and amount, or its quick-add values
// @Tags        nutrition
// @Param       id path string true "Entry id"
// @Param       Idempotency-Key header string true "Equal to operation_key"
// @Param       body body model.UpdateFoodEntryRequest true "Change with expected_revision"
// @Success     200 {object} model.FoodEntryUpdated
// @Failure     404 {object} map[string]string
// @Failure     409 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/entries/{id} [put]
func (h *FoodHandler) UpdateEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := foodIDParam(w, r, "food entry not found")
	if !ok {
		return
	}
	var req model.UpdateFoodEntryRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	updated, _, err := h.svc.UpdateEntry(r.Context(), middleware.MustGetUserID(r.Context()), id, req)
	respond(w, updated, err, http.StatusOK)
}

// DeleteEntry godoc
// @Summary     Remove an entry; removing the last one removes the day's totals
// @Tags        nutrition
// @Param       id path string true "Entry id"
// @Param       expected_revision query int true "Current revision"
// @Param       Idempotency-Key header string true "Operation key"
// @Success     200 {object} model.FoodEntryDeleted
// @Failure     404 {object} map[string]string
// @Failure     409 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/entries/{id} [delete]
func (h *FoodHandler) DeleteEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := foodIDParam(w, r, "food entry not found")
	if !ok {
		return
	}
	revision, key, ok := deleteParams(w, r)
	if !ok {
		return
	}
	deleted, _, err := h.svc.DeleteEntry(r.Context(), middleware.MustGetUserID(r.Context()), id, revision, key)
	respond(w, deleted, err, http.StatusOK)
}

// foodIDParam parses a UUID path parameter; a malformed id is the shared
// not-found so ownership and existence stay indistinguishable.
func foodIDParam(w http.ResponseWriter, r *http.Request, message string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, message, "NOT_FOUND", nil)
		return uuid.Nil, false
	}
	return id, true
}

// deleteParams reads the revision and operation key a body-less delete
// carries in the query string and the Idempotency-Key header.
func deleteParams(w http.ResponseWriter, r *http.Request) (int64, string, bool) {
	revision, err := strconv.ParseInt(r.URL.Query().Get("expected_revision"), 10, 64)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "expected_revision must be a whole number", "VALIDATION_ERROR", map[string]string{"field": "expected_revision"})
		return 0, "", false
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		writeError(w, http.StatusUnprocessableEntity, "Idempotency-Key is required", "VALIDATION_ERROR", map[string]string{"field": "Idempotency-Key"})
		return 0, "", false
	}
	return revision, key, true
}
