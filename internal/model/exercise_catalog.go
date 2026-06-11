package model

import "github.com/google/uuid"

// CatalogExercise is a curated entry in the exercise catalog.
// Mechanic is nil for cardio entries (and serialized as JSON null).
type CatalogExercise struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Category  string    `json:"category"`
	Modality  string    `json:"modality"`
	Mechanic  *string   `json:"mechanic"`
	SortOrder int       `json:"sort_order"`
}
