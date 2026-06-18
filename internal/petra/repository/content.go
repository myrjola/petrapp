package repository

import (
	"encoding/json"
	"fmt"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

// exerciseContent is the on-disk shape of the exercises.content JSON column:
// the structured instructional content that replaced the former free-form
// Markdown description. It is a value object owned by the Exercise aggregate —
// never queried independently — so it lives inline as one JSON column rather
// than in child tables.
type exerciseContent struct {
	Instructions   []string          `json:"instructions"`
	CommonMistakes []string          `json:"common_mistakes"`
	Resources      []domain.Resource `json:"resources"`
}

// marshalExerciseContent serialises an exercise's structured content for the
// content column. Nil slices are normalised to empty so the stored JSON always
// carries the three keys.
func marshalExerciseContent(ex domain.Exercise) (string, error) {
	content := exerciseContent{
		Instructions:   ex.Instructions,
		CommonMistakes: ex.CommonMistakes,
		Resources:      ex.Resources,
	}
	if content.Instructions == nil {
		content.Instructions = []string{}
	}
	if content.CommonMistakes == nil {
		content.CommonMistakes = []string{}
	}
	if content.Resources == nil {
		content.Resources = []domain.Resource{}
	}
	b, err := json.Marshal(content)
	if err != nil {
		return "", fmt.Errorf("marshal exercise content: %w", err)
	}
	return string(b), nil
}

// unmarshalExerciseContent decodes the content column onto ex. An empty string
// (the column DEFAULT before any write) yields empty slices.
func unmarshalExerciseContent(raw string, ex *domain.Exercise) error {
	if raw == "" {
		return nil
	}
	var content exerciseContent
	if err := json.Unmarshal([]byte(raw), &content); err != nil {
		return fmt.Errorf("unmarshal exercise content: %w", err)
	}
	ex.Instructions = content.Instructions
	ex.CommonMistakes = content.CommonMistakes
	ex.Resources = content.Resources
	return nil
}
