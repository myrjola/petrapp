package service

// This file owns the OpenAI-driven generator that fills in a freshly
// named exercise's metadata (category, type, muscle groups, description,
// resources). The decision tree in generateExerciseContent prefers the
// AI path; on any failure (missing API key, network error, malformed
// response, schema validation failure) it falls back to a minimal
// exercise so the user can edit the rest by hand. GenerateExercise
// persists whichever exercise was produced.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/myrjola/petrapp/internal/petra/domain"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

// resourceURLValidationTimeout caps each probe issued by validateResourceURLs.
const resourceURLValidationTimeout = 5 * time.Second

// resourceProbeUserAgent identifies our link-validation probes. A default
// Go-http-client User-Agent is frequently rejected outright; a descriptive
// one fares better while staying honest about who is calling.
const resourceProbeUserAgent = "PetraBot/1.0 (+https://github.com/myrjola/petrapp)"

// exerciseJSONSchema is the JSON-schema description that the OpenAI
// chat completion endpoint validates the AI's response against. The
// muscle-group enum is dynamic — the generator constructs the schema
// per call with the muscle groups the database currently exposes so
// the AI can never invent ones we don't track.
type exerciseJSONSchema struct {
	muscleGroups []string
}

// JSON Schema keys ("type", "description", "string", "enum") are spec-fixed strings.
//

func (ejs exerciseJSONSchema) MarshalJSON() ([]byte, error) {
	result, err := json.Marshal(ejs.schemaMap())
	if err != nil {
		return nil, fmt.Errorf("marshal exercise schema: %w", err)
	}
	return result, nil
}

// schemaMap builds the JSON-schema object as a map so it can be passed
// directly to the Responses API's structured-output Schema field, which is
// typed map[string]any. MarshalJSON marshals this same map.
func (ejs exerciseJSONSchema) schemaMap() map[string]any {
	return map[string]any{
		"type": "object", //nolint:goconst // JSON Schema keyword, not a shared constant.
		"required": []string{
			"id",
			"name",
			"category",
			"exercise_type",
			"default_starting_seconds",
			"description_markdown",
			"primary_muscle_groups",
			"secondary_muscle_groups",
		},
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "integer",
				"description": "Unique identifier for the exercise, leave as -1 for new exercises", //nolint:goconst // JSON Schema keyword, not a shared constant.
			},
			"name": map[string]any{
				"type":        "string", //nolint:goconst // JSON Schema type value, not a shared constant.
				"description": "Name of the exercise",
			},
			"category": map[string]any{
				"type":        "string",
				"description": "Category of the exercise",
				"enum": []string{ //nolint:goconst // JSON Schema keyword, not a shared constant.
					"full_body",
					"upper",
					"lower",
				},
			},
			"exercise_type": map[string]any{
				"type":        "string",
				"description": "Type of exercise: weighted, bodyweight, assisted, or time_based",
				"enum":        []string{"weighted", "bodyweight", "assisted", "time_based"},
			},
			"default_starting_seconds": map[string]any{
				"type":        []string{"integer", "null"},
				"description": "Default starting seconds for time_based exercises; null for other types",
			},
			"description_markdown": map[string]any{
				"type":        "string",
				"description": "Markdown description of the exercise",
			},
			"primary_muscle_groups": map[string]any{
				"type":        "array",
				"description": "Primary muscle groups targeted by the exercise",
				"items": map[string]any{
					"type": "string",
					"enum": ejs.muscleGroups,
				},
			},
			"secondary_muscle_groups": map[string]any{
				"type":        "array",
				"description": "Secondary muscle groups targeted by the exercise",
				"items": map[string]any{
					"type": "string",
					"enum": ejs.muscleGroups,
				},
			},
		},
		"additionalProperties": false,
	}
}

// exerciseGenerator generates exercises using OpenAI API.
type exerciseGenerator struct {
	client       openai.Client
	httpClient   *http.Client
	logger       *slog.Logger
	muscleGroups []string
}

// newExerciseGenerator creates a new exercise generator.
func newExerciseGenerator(openaiAPIKey string, muscleGroups []string, logger *slog.Logger) *exerciseGenerator {
	client := openai.NewClient(option.WithAPIKey(openaiAPIKey))
	return &exerciseGenerator{
		client:       client,
		httpClient:   &http.Client{Timeout: resourceURLValidationTimeout},
		logger:       logger,
		muscleGroups: muscleGroups,
	}
}

// Generate generates a new exercise based on the given name.
func (eg *exerciseGenerator) Generate(ctx context.Context, name string) (domain.Exercise, error) {
	if name == "" {
		return domain.Exercise{}, errors.New("exercise name cannot be empty")
	}

	// Pass 1: Generate exercise with placeholder URLs
	exercise, err := eg.generateBaseExercise(ctx, name)
	if err != nil {
		return domain.Exercise{}, err
	}

	// Pass 2: Enhance with real URLs from web search (non-blocking failure)
	if err = eg.enhanceWithWebSearch(ctx, &exercise); err != nil {
		eg.logger.LogAttrs(ctx, slog.LevelWarn, "failed to enhance exercise with web search", slog.Any("error", err))
	}

	return exercise, nil
}

// baseExercisePrompt builds the user prompt sent to the chat completion endpoint
// for an exercise of the given name. The exercise_type enum values listed here
// must stay in sync with the schema's exercise_type enum.
func (eg *exerciseGenerator) baseExercisePrompt(name string) string {
	return fmt.Sprintf(`Generate a detailed exercise for "%s".

The response must strictly follow this JSON structure:
{
  "id": -1,
  "name": "%s",
  "category": "CATEGORY",
  "exercise_type": "EXERCISE_TYPE",
  "default_starting_seconds": 30,
  "description_markdown": "MARKDOWN_DESCRIPTION",
  "primary_muscle_groups": ["PRIMARY_MUSCLE_GROUP1", "PRIMARY_MUSCLE_GROUP2"],
  "secondary_muscle_groups": ["SECONDARY_MUSCLE_GROUP1", "SECONDARY_MUSCLE_GROUP2"]
}

For "category", use one of: "full_body", "upper", "lower"
For "exercise_type", use one of: "weighted", "bodyweight", "assisted", "time_based"
  - Use "time_based" for isometric holds and timed exercises (planks, wall sits, dead hangs, etc.)
  - Use "weighted" for exercises performed with external load
  - Use "bodyweight" for exercises performed against gravity alone
  - Use "assisted" for exercises that reduce bodyweight (assisted pull-ups, etc.)
For "default_starting_seconds", set a reasonable beginner duration in seconds (e.g. 20-45)
when exercise_type is "time_based"; otherwise set it to null.
For "muscle_groups", use only from this list: %s

Muscle-group rule: only credit a muscle as primary or secondary if it performs a
working contraction (concentric or eccentric load). Pure isometric stabilizers
(e.g. the lats during a push-up, the upper back during a bench press, the core
during an overhead press) do not count and must be omitted.

The "description_markdown" must follow this exact structure:

## Instructions
1. [Step 1 with clear form guidance]
2. [Step 2 with positioning details]
3. [Step 3 with movement description]
4. [Optional step 4 with breathing/tempo guidance]

## Common Mistakes
- [Mistake 1: explanation of error and correction]
- [Mistake 2: explanation of error and correction]
- [Mistake 3: explanation of error and correction]
- [Optional Mistake 4: explanation of error and correction]

Description content rules:
- Do not include rep counts, set counts, weights, or durations anywhere in the
  description. The app tracks rep and set targets separately and shows them to
  the user. Mentions like "perform 8-12 reps", "do 3 sets", or "hold for 30
  seconds" must not appear.
- Do not include a Resources section. Tutorial links are added by a
  follow-up search step and appended automatically.

Instructions must be clear, concise, and focus on proper form using simple language for beginners.
Include relevant safety considerations. The entire description should be 150-200 words.

Return only the valid JSON object with no additional text or explanation.`,
		name, name, strings.Join(eg.muscleGroups, ", "))
}

// generateBaseExercise creates the base exercise structure with placeholder URLs.
func (eg *exerciseGenerator) generateBaseExercise(ctx context.Context, name string) (domain.Exercise, error) {
	prompt := eg.baseExercisePrompt(name)

	// Query the Responses API with strict structured-output JSON schema.
	resp, err := eg.client.Responses.New(ctx,
		responses.ResponseNewParams{
			Model: openai.ChatModelGPT5_4,
			Input: responses.ResponseNewParamsInputUnion{
				OfString: openai.String(prompt),
			},
			Text: responses.ResponseTextConfigParam{
				Format: responses.ResponseFormatTextConfigUnionParam{
					OfJSONSchema: &responses.ResponseFormatTextJSONSchemaConfigParam{
						Name:        "exercise",
						Description: openai.String("Detailed information about a fitness exercise"),
						Schema:      exerciseJSONSchema{muscleGroups: eg.muscleGroups}.schemaMap(),
						Strict:      openai.Bool(true),
					},
				},
			},
		})

	if err != nil {
		return domain.Exercise{}, fmt.Errorf("responses completion: %w", err)
	}

	// Parse the response
	var exercise domain.Exercise
	err = json.Unmarshal([]byte(resp.OutputText()), &exercise)
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("parse exercise response: %w", err)
	}

	// Validate the exercise
	if exercise.Name == "" || exercise.Category == "" || exercise.DescriptionMarkdown == "" {
		return domain.Exercise{}, errors.New("generated exercise is missing required fields")
	}

	// Verify muscle groups
	if len(exercise.PrimaryMuscleGroups) == 0 {
		return domain.Exercise{}, errors.New("generated exercise has no primary muscle groups")
	}

	muscleGroups := slices.Concat(exercise.PrimaryMuscleGroups, exercise.SecondaryMuscleGroups)
	if err = eg.validateMuscleGroups(muscleGroups); err != nil {
		return domain.Exercise{}, fmt.Errorf("validate muscle groups: %w", err)
	}

	return exercise, nil
}

// enhanceWithWebSearch enriches the exercise description with real tutorial links.
func (eg *exerciseGenerator) enhanceWithWebSearch(ctx context.Context, exercise *domain.Exercise) error {
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

	// Attach the built-in web_search tool so the model returns real, live URLs
	// rather than ones recalled from training data.
	resp, err := eg.client.Responses.New(ctx,
		responses.ResponseNewParams{
			Model: openai.ChatModelGPT5_4,
			Input: responses.ResponseNewParamsInputUnion{
				OfString: openai.String(prompt),
			},
			Tools: []responses.ToolUnionParam{
				{OfWebSearch: &responses.WebSearchToolParam{
					Type: responses.WebSearchToolTypeWebSearch,
				}},
			},
		})

	if err != nil {
		return fmt.Errorf("web search completion: %w", err)
	}

	// Parse resources from response. With a hosted tool in play the model may
	// wrap the JSON in prose or a markdown code fence, so extract the object
	// before unmarshalling.
	var resourceResponse struct {
		Resources []domain.Resource `json:"resources"`
	}
	err = json.Unmarshal([]byte(extractJSONObject(resp.OutputText())), &resourceResponse)
	if err != nil {
		return fmt.Errorf("parse resources response: %w", err)
	}

	// Validate URLs before injecting them: drop dead links so the
	// description never ships with broken Resources entries.
	alive := eg.validateResourceURLs(ctx, resourceResponse.Resources)
	if len(alive) == 0 && len(resourceResponse.Resources) > 0 {
		eg.logger.LogAttrs(ctx, slog.LevelInfo, "dropped all resource URLs",
			slog.String("exercise", exercise.Name),
			slog.Int("returned", len(resourceResponse.Resources)))
	}
	exercise.DescriptionMarkdown = eg.updateResourcesInDescription(
		exercise.DescriptionMarkdown,
		alive,
	)

	return nil
}

// accessRestrictedStatus reports whether status means the server is up and the
// resource almost certainly exists but is refusing our automated probe
// (unauthorized, forbidden, method-not-allowed, rate-limited). Reputable sites
// such as Mayo Clinic and NASM return 403/405 to HEAD requests from
// non-browser clients, so treating these as dead would discard good tutorial
// links.
func accessRestrictedStatus(status int) bool {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden,
		http.StatusMethodNotAllowed, http.StatusTooManyRequests:
		return true
	default:
		return false
	}
}

// probeResourceURL issues a single request with the descriptive probe
// User-Agent and returns the response status code. The body is closed
// immediately (we never need it). A non-nil error means the request never
// completed: malformed URL, network failure, or timeout.
func (eg *exerciseGenerator) probeResourceURL(
	ctx context.Context,
	method, url string,
) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return 0, fmt.Errorf("build %s request: %w", method, err)
	}
	req.Header.Set("User-Agent", resourceProbeUserAgent)
	resp, err := eg.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%s %s: %w", method, url, err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

// validateResourceURLs probes each resource URL with the generator's http
// client and returns the subset that is reachable. It HEAD-checks first; when
// a server answers with an access-restricted status (403/405/etc.) — common
// for sites that block HEAD or non-browser clients — it retries once with GET
// before deciding. 2xx/3xx responses are kept; access-restricted responses are
// kept (the link exists, the server just won't let us probe it); genuine 4xx
// (404/410) and 5xx responses, and network failures, are logged at debug level
// and dropped. Best-effort: never returns an error.
func (eg *exerciseGenerator) validateResourceURLs(
	ctx context.Context,
	resources []domain.Resource,
) []domain.Resource {
	alive := make([]domain.Resource, 0, len(resources))
	for _, r := range resources {
		status, err := eg.probeResourceURL(ctx, http.MethodHead, r.URL)
		if err != nil {
			eg.logger.LogAttrs(ctx, slog.LevelDebug, "skip resource: request failed",
				slog.String("url", r.URL), slog.Any("error", err))
			continue
		}
		// A HEAD rejection may just mean the server dislikes HEAD or bots;
		// a GET often succeeds where HEAD did not.
		if accessRestrictedStatus(status) {
			if getStatus, getErr := eg.probeResourceURL(ctx, http.MethodGet, r.URL); getErr == nil {
				status = getStatus
			}
		}
		switch {
		case status < http.StatusBadRequest:
			alive = append(alive, r)
		case accessRestrictedStatus(status):
			eg.logger.LogAttrs(ctx, slog.LevelDebug, "keep resource: probe restricted",
				slog.String("url", r.URL), slog.Int("status", status))
			alive = append(alive, r)
		default:
			eg.logger.LogAttrs(ctx, slog.LevelDebug, "skip resource: bad status",
				slog.String("url", r.URL), slog.Int("status", status))
		}
	}
	return alive
}

// updateResourcesInDescription replaces placeholder URLs with real ones.
// When resources is empty the ## Resources section is dropped entirely so no
// orphan heading is left behind.
func (eg *exerciseGenerator) updateResourcesInDescription(
	markdown string,
	resources []domain.Resource,
) string {
	lines := strings.Split(markdown, "\n")
	var result []string
	inResourcesSection := false

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "## Resources"):
			inResourcesSection = true
			if len(resources) == 0 {
				continue
			}
			result = append(result, line)
			for _, res := range resources {
				result = append(result, fmt.Sprintf("- [%s](%s)", res.Title, res.URL))
			}
		case inResourcesSection && strings.HasPrefix(line, "##"):
			inResourcesSection = false
			result = append(result, line)
		case !inResourcesSection:
			result = append(result, line)
		}
	}

	// If no Resources section was present and we have resources to add, append one.
	if !inResourcesSection && len(resources) > 0 && !containsResourcesHeading(result) {
		result = append(result, "\n## Resources")
		for _, res := range resources {
			result = append(result, fmt.Sprintf("- [%s](%s)", res.Title, res.URL))
		}
	}

	return strings.Join(result, "\n")
}

// containsResourcesHeading reports whether any line in result already starts
// with "## Resources". Used by updateResourcesInDescription to avoid emitting
// a duplicate section when the input already had one and it was replaced.
func containsResourcesHeading(lines []string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, "## Resources") {
			return true
		}
	}
	return false
}

// extractJSONObject returns the first complete top-level JSON object found in
// s. It tolerates a surrounding markdown code fence (```json ... ```) and any
// leading or trailing prose the model may emit alongside the object when a
// hosted tool such as web search is active. When no object is found it returns
// the trimmed input unchanged so the caller's json.Unmarshal surfaces the
// original parse error.
func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return strings.TrimSpace(s)
	}
	// Walk braces, ignoring those inside string literals, to find the match.
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case escaped:
			escaped = false
		case c == '\\' && inString:
			escaped = true
		case c == '"':
			inString = !inString
		case inString:
			// other characters inside a string are literal
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return strings.TrimSpace(s)
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

// GenerateExercise generates a new exercise based on a name.
//
// In case of errors, it persists a minimal exercise that the user can fill in later.
// The returned exercise is guaranteed to have at least Name and ID fields set.
func (s *Service) GenerateExercise(ctx context.Context, name string) (domain.Exercise, error) {
	if name == "" {
		return domain.Exercise{}, domain.ValidationError{Message: "Exercise name is required."}
	}
	exercise := s.generateExerciseContent(ctx, name)

	persisted, err := s.repos.Exercises.Create(ctx, exercise)
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("create exercise: %w", err)
	}

	return persisted, nil
}

// generateExerciseContent creates exercise content, using AI generation if available
// or falling back to minimal content if not possible.
func (s *Service) generateExerciseContent(ctx context.Context, name string) domain.Exercise {
	if s.openaiAPIKey == "" {
		return createMinimalExercise(name)
	}

	muscleGroups, err := s.repos.Exercises.ListMuscleGroups(ctx)
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "failed to get muscle groups", slog.Any("error", err))
		return createMinimalExercise(name)
	}

	generator := newExerciseGenerator(s.openaiAPIKey, muscleGroups, s.logger)
	generated, err := generator.Generate(ctx, name)
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "failed to generate exercise details",
			slog.Any("error", err), slog.String("name", name))
		return createMinimalExercise(name)
	}

	// Defensive default: the AI prompt does not carry rep_min/rep_max, and
	// the DB CHECK requires them for non-time-based exercises. Mirror the
	// values used by createMinimalExercise so the Create downstream succeeds.
	if generated.ExerciseType != domain.ExerciseTypeTime &&
		(generated.RepMin == nil || generated.RepMax == nil) {
		repMin, repMax := 5, 10
		generated.RepMin = &repMin
		generated.RepMax = &repMax
	}
	return generated
}

// createMinimalExercise returns a basic exercise with just the essential fields populated.
func createMinimalExercise(name string) domain.Exercise {
	repMin, repMax := 5, 10
	return domain.Exercise{ //nolint:exhaustruct // DefaultStartingSeconds is nil for non-time_based exercises.
		ID:                    -1,
		Name:                  name,
		Category:              domain.CategoryFullBody,
		ExerciseType:          domain.ExerciseTypeWeighted,
		DescriptionMarkdown:   fmt.Sprintf("# %s\n\nNo description available yet.", name),
		PrimaryMuscleGroups:   []string{},
		SecondaryMuscleGroups: []string{},
		RepMin:                &repMin,
		RepMax:                &repMax,
	}
}
