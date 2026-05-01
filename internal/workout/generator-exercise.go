package workout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// exerciseGenerator generates exercises using OpenAI API.
type exerciseGenerator struct {
	client       openai.Client
	logger       *slog.Logger
	muscleGroups []string
}

// newExerciseGenerator creates a new exercise generator.
func newExerciseGenerator(openaiAPIKey string, muscleGroups []string, logger *slog.Logger) *exerciseGenerator {
	client := openai.NewClient(option.WithAPIKey(openaiAPIKey))
	return &exerciseGenerator{
		client:       client,
		logger:       logger,
		muscleGroups: muscleGroups,
	}
}

// Generate generates a new exercise based on the given name.
func (eg *exerciseGenerator) Generate(ctx context.Context, name string) (Exercise, error) {
	if name == "" {
		return Exercise{}, errors.New("exercise name cannot be empty")
	}

	// Pass 1: Generate exercise with placeholder URLs
	exercise, err := eg.generateBaseExercise(ctx, name)
	if err != nil {
		return Exercise{}, err
	}

	// Pass 2: Enhance with real URLs from web search (non-blocking failure)
	if err = eg.enhanceWithWebSearch(ctx, &exercise); err != nil {
		eg.logger.LogAttrs(ctx, slog.LevelWarn, "failed to enhance exercise with web search", slog.Any("error", err))
	}

	return exercise, nil
}

// generateBaseExercise creates the base exercise structure with placeholder URLs.
func (eg *exerciseGenerator) generateBaseExercise(ctx context.Context, name string) (Exercise, error) {
	prompt := fmt.Sprintf(`Generate a detailed exercise for "%s".

The response must strictly follow this JSON structure:
{
  "id": -1,
  "name": "%s",
  "category": "CATEGORY",
  "exercise_type": "EXERCISE_TYPE",
  "description_markdown": "MARKDOWN_DESCRIPTION",
  "primary_muscle_groups": ["PRIMARY_MUSCLE_GROUP1", "PRIMARY_MUSCLE_GROUP2"],
  "secondary_muscle_groups": ["SECONDARY_MUSCLE_GROUP1", "SECONDARY_MUSCLE_GROUP2"]
}

For "category", use one of: "full_body", "upper", "lower"
For "exercise_type", use one of: "weighted", "bodyweight", "assisted"
For "muscle_groups", use only from this list: %s

The "description_markdown" must follow this exact structure:

## Instructions
1. [Step 1 with clear form guidance]
2. [Step 2 with positioning details]
3. [Step 3 with movement description]
4. [Optional step 4 with breathing/tempo guidance]
5. [Optional step 5 with repetition guidance]

## Common Mistakes
- [Mistake 1: explanation of error and correction]
- [Mistake 2: explanation of error and correction]
- [Mistake 3: explanation of error and correction]
- [Optional Mistake 4: explanation of error and correction]

## Resources
- [Video tutorial](https://example.com/exercise-video)
- [Form guide](https://example.com/exercise-form)
- [Optional additional resource](https://example.com/exercise-variations)

Instructions must be clear, concise, and focus on proper form using simple language for beginners.
Include relevant safety considerations. The entire description should be 150-200 words.

Return only the valid JSON object with no additional text or explanation.`,
		name, name, strings.Join(eg.muscleGroups, ", "))

	schemaParam := openai.ResponseFormatJSONSchemaJSONSchemaParam{
		Name:        "exercise",
		Description: openai.Opt("Detailed information about a fitness exercise"),
		Schema:      openai.Opt(any(exerciseJSONSchema{muscleGroups: eg.muscleGroups})),
		Strict:      openai.Bool(true),
	}

	// Query the OpenAI API with strict JSON mode
	chat, err := eg.client.Chat.Completions.New(ctx,
		openai.ChatCompletionNewParams{
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage(prompt),
			},
			ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
				OfText: nil,
				OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
					Type:       "json_schema",
					JSONSchema: schemaParam,
				},
				OfJSONObject: nil,
			},
			Model: openai.ChatModelGPT5_4,
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

// enhanceWithWebSearch enriches the exercise description with real tutorial links.
func (eg *exerciseGenerator) enhanceWithWebSearch(ctx context.Context, exercise *Exercise) error {
	prompt := fmt.Sprintf(`Find the best fitness tutorial resources for "%s" exercise.

Search for:
1. A YouTube video tutorial showing proper form
2. A detailed form guide or article
3. An optional supplementary resource (variations, common mistakes guide, etc.)

Return a JSON object with exactly this structure:
{
  "resources": [
    {"title": "Video Title", "url": "https://..."},
    {"title": "Article Title", "url": "https://..."},
    {"title": "Optional Resource Title", "url": "https://..."}
  ]
}

Requirements:
- URLs must be complete and valid
- Prioritize YouTube for videos, fitness sites like ExRx.net, NASM, ACE for guides
- Only include real, relevant resources you find through search
- Return empty array if search yields no results

Return only the JSON object.`, exercise.Name)

	// Use non-strict mode to enable web search
	chat, err := eg.client.Chat.Completions.New(ctx,
		openai.ChatCompletionNewParams{
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage(prompt),
			},
			Model: openai.ChatModelGPT5_4,
		})

	if err != nil {
		return fmt.Errorf("web search completion: %w", err)
	}

	// Parse resources from response
	var resourceResponse struct {
		Resources []Resource `json:"resources"`
	}
	err = json.Unmarshal([]byte(chat.Choices[0].Message.Content), &resourceResponse)
	if err != nil {
		return fmt.Errorf("parse resources response: %w", err)
	}

	// Update description with real URLs if found
	if len(resourceResponse.Resources) > 0 {
		exercise.DescriptionMarkdown = eg.updateResourcesInDescription(
			exercise.DescriptionMarkdown,
			resourceResponse.Resources,
		)
	}

	return nil
}

// updateResourcesInDescription replaces placeholder URLs with real ones.
func (eg *exerciseGenerator) updateResourcesInDescription(
	markdown string,
	resources []Resource,
) string {
	// Find and replace the Resources section
	lines := strings.Split(markdown, "\n")
	var result []string
	inResourcesSection := false

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "## Resources"):
			inResourcesSection = true
			result = append(result, line)
			for _, res := range resources {
				result = append(result, fmt.Sprintf("- [%s](%s)", res.Title, res.URL))
			}
		case inResourcesSection && strings.HasPrefix(line, "##"):
			inResourcesSection = false
			result = append(result, line)
		case !inResourcesSection:
			if !strings.HasPrefix(line, "- [") || !inResourcesSection {
				result = append(result, line)
			}
		}
	}

	// If no resources section found, append one
	if !inResourcesSection && len(resources) > 0 {
		result = append(result, "\n## Resources")
		for _, res := range resources {
			result = append(result, fmt.Sprintf("- [%s](%s)", res.Title, res.URL))
		}
	}

	return strings.Join(result, "\n")
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
