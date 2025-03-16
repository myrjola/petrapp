package workout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// exerciseGenerator generates exercises using OpenAI API.
type exerciseGenerator struct {
	client       *openai.Client
	muscleGroups []string
}

// newExerciseGenerator creates a new exercise generator.
func newExerciseGenerator(openaiAPIKey string, muscleGroups []string) *exerciseGenerator {
	client := openai.NewClient(option.WithAPIKey(openaiAPIKey))
	return &exerciseGenerator{
		client:       client,
		muscleGroups: muscleGroups,
	}
}

// Generate generates a new exercise based on the given name.
func (eg *exerciseGenerator) Generate(ctx context.Context, name string) (Exercise, error) {
	if name == "" {
		return Exercise{}, errors.New("exercise name cannot be empty")
	}

	prompt := fmt.Sprintf(`Generate a detailed exercise description for "%s". 
Include the appropriate category (full_body, upper, or lower), 
and the primary and secondary muscle groups it targets, and
a markdown description following this exact structure:

## Instructions
[Provide 3-5 numbered steps explaining how to perform the exercise correctly]

## Common Mistakes
[List 3-4 common form errors as bullet points]

## Resources
[Include 2-3 placeholder links for videos and guides]

Important guidelines:
- Instructions should be clear, concise, and focus on proper form
- Use simple, direct language that beginners can understand
- Highlight safety considerations where relevant
- For the Resources section, use placeholder URLs (https://example.com/resource-name)

The description should be comprehensive yet concise, totaling around 150-200 words.`, name)

	schemaParam := openai.ResponseFormatJSONSchemaJSONSchemaParam{
		Name:        openai.F("exercise"),
		Description: openai.F("Detailed information about a fitness exercise"),
		Schema:      openai.F(interface{}(exerciseJSONSchema{muscleGroups: eg.muscleGroups})),
		Strict:      openai.Bool(true),
	}

	// Query the OpenAI API
	chat, err := eg.client.Chat.Completions.New(ctx,
		openai.ChatCompletionNewParams{ //nolint:exhaustruct // only need to set a few fields.
			Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
				openai.UserMessage(prompt),
			}),
			ResponseFormat: openai.F[openai.ChatCompletionNewParamsResponseFormatUnion](
				openai.ResponseFormatJSONSchemaParam{
					Type:       openai.F(openai.ResponseFormatJSONSchemaTypeJSONSchema),
					JSONSchema: openai.F(schemaParam),
				},
			),
			Model: openai.F(openai.ChatModelGPT4o2024_08_06),
		})

	if err != nil {
		return Exercise{}, fmt.Errorf("chat completion: %w", err)
	}

	// Parse the response
	var exercise Exercise
	err = json.Unmarshal([]byte(chat.Choices[0].Message.Content), &exercise)
	if err != nil {
		return Exercise{}, fmt.Errorf("parse exercise response: %w", err)
	}

	// Validate the exercise
	if exercise.Name == "" || exercise.Category == "" || exercise.DescriptionMarkdown == "" {
		return Exercise{}, errors.New("generated exercise is missing required fields")
	}

	// Verify muscle groups
	if len(exercise.PrimaryMuscleGroups) == 0 {
		return Exercise{}, errors.New("generated exercise has no primary muscle groups")
	}

	muscleGroups := slices.Concat(exercise.PrimaryMuscleGroups, exercise.SecondaryMuscleGroups)
	if err = eg.validateMuscleGroups(muscleGroups); err != nil {
		return Exercise{}, fmt.Errorf("validate muscle groups: %w", err)
	}

	return exercise, nil
}

// validateMuscleGroups checks if all muscle groups are in the allowed list.
func (eg *exerciseGenerator) validateMuscleGroups(groups []string) error {
	if len(groups) == 0 {
		return nil
	}

	for _, group := range groups {
		if !slices.Contains(eg.muscleGroups, group) {
			return fmt.Errorf("invalid muscle group %q", group)
		}
	}

	return nil
}
