package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// DefaultModel is used whenever no gemini_model setting has been configured.
// Confirmed against Google's live model list (ai.google.dev/gemini-api/docs/models)
// rather than assumed from training data, since model names move fast —
// re-verify before bumping this if it looks stale.
const DefaultModel = "gemini-3.6-flash"

// Client is the seam between internal/inventory and the actual Gemini API.
// Every method takes an already-resolved model and prompt (default or
// user-overridden — see internal/gemini.DefaultPrompt and the settings
// table) so this package stays free of any settings/DB dependency.
type Client interface {
	// AnalyzeTagCapture reads a single captured photo for an asset tag,
	// item identity, and per-image notes (README flow #1).
	AnalyzeTagCapture(ctx context.Context, model, prompt string, image []byte, mimeType string) (TagCaptureResult, error)

	// AnalyzeReconciliation reads a single captured photo for a location
	// code and the asset tags visible alongside it (README flow #2).
	AnalyzeReconciliation(ctx context.Context, model, prompt string, image []byte, mimeType string) (ReconciliationResult, error)

	// RegenerateDescription consolidates an item's per-image notes into one
	// description (the Search view's bulk "Regenerate description" action).
	// hint is an optional user-supplied steer (e.g. "blue enclosure"); pass
	// "" when there is none.
	RegenerateDescription(ctx context.Context, model, prompt string, assetTag string, perImageDescriptions []string, hint string) (DescriptionResult, error)

	// DetectDuplicates scans every item's asset tag + description for
	// likely duplicates (the Duplicate finder, README flow #5).
	DetectDuplicates(ctx context.Context, model, prompt string, items []AssetTagDescription) (DuplicateDetectionResult, error)
}

type GenAIClient struct {
	client *genai.Client
}

// NewGenAIClient constructs a real Gemini-backed Client. Returns an error if
// apiKey is empty — callers should treat a missing GEMINI_API_KEY as "AI
// features disabled" rather than crash-looping the whole server (see main.go).
func NewGenAIClient(ctx context.Context, apiKey string) (*GenAIClient, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("gemini: empty API key")
	}
	c, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		return nil, fmt.Errorf("gemini: create client: %w", err)
	}
	return &GenAIClient{client: c}, nil
}

var tagCaptureSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"has_asset_tag": {Type: genai.TypeBoolean},
		"asset_tag":     {Type: genai.TypeString},
		"item_guess":    {Type: genai.TypeString},
		"description":   {Type: genai.TypeString},
	},
	Required: []string{"has_asset_tag", "asset_tag", "item_guess", "description"},
}

func (g *GenAIClient) AnalyzeTagCapture(ctx context.Context, model, prompt string, image []byte, mimeType string) (TagCaptureResult, error) {
	var out TagCaptureResult
	err := g.generateJSON(ctx, model, tagCaptureSchema, &out,
		genai.NewPartFromText(prompt),
		genai.NewPartFromBytes(image, mimeType),
	)
	return out, err
}

var reconciliationSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"has_location_tag": {Type: genai.TypeBoolean},
		"location_tag":     {Type: genai.TypeString},
		"asset_tags":       {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
		"suggested_rotation": {
			Type:   genai.TypeString,
			Format: "enum",
			Enum:   []string{"clockwise", "counterclockwise"},
		},
	},
	Required: []string{"has_location_tag", "location_tag", "asset_tags", "suggested_rotation"},
}

func (g *GenAIClient) AnalyzeReconciliation(ctx context.Context, model, prompt string, image []byte, mimeType string) (ReconciliationResult, error) {
	var out ReconciliationResult
	err := g.generateJSON(ctx, model, reconciliationSchema, &out,
		genai.NewPartFromText(prompt),
		genai.NewPartFromBytes(image, mimeType),
	)
	return out, err
}

var descriptionSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"description": {Type: genai.TypeString},
	},
	Required: []string{"description"},
}

func (g *GenAIClient) RegenerateDescription(ctx context.Context, model, prompt string, assetTag string, perImageDescriptions []string, hint string) (DescriptionResult, error) {
	var body strings.Builder
	fmt.Fprintf(&body, "Asset tag: %s\n\nPer-photo notes:\n", assetTag)
	for _, d := range perImageDescriptions {
		if strings.TrimSpace(d) == "" {
			continue
		}
		fmt.Fprintf(&body, "- %s\n", d)
	}
	if strings.TrimSpace(hint) != "" {
		fmt.Fprintf(&body, "\nUser-supplied hint: %s\n", hint)
	}

	var out DescriptionResult
	err := g.generateJSON(ctx, model, descriptionSchema, &out,
		genai.NewPartFromText(prompt),
		genai.NewPartFromText(body.String()),
	)
	return out, err
}

var duplicateDetectionSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"groups": {
			Type: genai.TypeArray,
			Items: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"asset_tags": {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
					"reasoning":  {Type: genai.TypeString},
				},
				Required: []string{"asset_tags", "reasoning"},
			},
		},
	},
	Required: []string{"groups"},
}

func (g *GenAIClient) DetectDuplicates(ctx context.Context, model, prompt string, items []AssetTagDescription) (DuplicateDetectionResult, error) {
	var body strings.Builder
	body.WriteString("Items:\n")
	for _, it := range items {
		fmt.Fprintf(&body, "- %s: %s\n", it.AssetTag, it.Description)
	}

	var out DuplicateDetectionResult
	err := g.generateJSON(ctx, model, duplicateDetectionSchema, &out,
		genai.NewPartFromText(prompt),
		genai.NewPartFromText(body.String()),
	)
	return out, err
}

func (g *GenAIClient) generateJSON(ctx context.Context, model string, schema *genai.Schema, out any, parts ...*genai.Part) error {
	content := genai.NewContentFromParts(parts, genai.RoleUser)

	resp, err := g.client.Models.GenerateContent(ctx, model, []*genai.Content{content}, &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema:   schema,
		// These calls read fixed facts off a photo (a tag, a code) or
		// consolidate existing text — never open-ended generation — so
		// sampling randomness only adds hallucination risk with no upside.
		Temperature: genai.Ptr[float32](0),
		TopK:        genai.Ptr[float32](1),
	})
	if err != nil {
		return fmt.Errorf("gemini: generate content: %w", err)
	}

	return parseJSONResponse(resp.Text(), out)
}

// parseJSONResponse decodes a model's structured-output text into out. Kept
// separate from generateJSON so response parsing is unit-testable against
// canned strings without a live API call.
func parseJSONResponse(text string, out any) error {
	if err := json.Unmarshal([]byte(text), out); err != nil {
		return fmt.Errorf("gemini: parse response %q: %w", text, err)
	}
	return nil
}
