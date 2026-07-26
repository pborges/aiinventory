package inventory

import (
	"context"
	"fmt"

	"github.com/pborges/aiinventory/internal/domain"
	"github.com/pborges/aiinventory/internal/gemini"
)

type DescriptionStore interface {
	GetItemByID(ctx context.Context, id int64) (domain.Item, error)
	ListImageMetaByItem(ctx context.Context, itemID int64) ([]domain.Image, error)
	UpdateItemDescription(ctx context.Context, id int64, description string) error
	LogActivity(ctx context.Context, userID int64, action domain.ActivityAction, itemID, locationID *int64, detail string) error
}

// RegenerateDescription implements the Search view's bulk "Regenerate
// description" action: Gemini reviews every per-image note attached to the
// item and consolidates them into one description, explicitly preserving
// serial/part numbers (see the description_regeneration default prompt).
func RegenerateDescription(ctx context.Context, s DescriptionStore, g gemini.Client, userID int64, model, prompt string, itemID int64) (string, error) {
	item, err := s.GetItemByID(ctx, itemID)
	if err != nil {
		return "", fmt.Errorf("look up item: %w", err)
	}

	images, err := s.ListImageMetaByItem(ctx, itemID)
	if err != nil {
		return "", fmt.Errorf("list images: %w", err)
	}
	notes := make([]string, 0, len(images))
	for _, img := range images {
		if img.Description != "" {
			notes = append(notes, img.Description)
		}
	}

	result, err := g.RegenerateDescription(ctx, model, prompt, item.AssetTag, notes)
	if err != nil {
		return "", fmt.Errorf("gemini: %w", err)
	}

	if err := s.UpdateItemDescription(ctx, itemID, result.Description); err != nil {
		return "", fmt.Errorf("update description: %w", err)
	}
	if err := s.LogActivity(ctx, userID, domain.ActivityDescriptionRegenerated, &itemID, nil, ""); err != nil {
		return "", fmt.Errorf("log activity: %w", err)
	}

	return result.Description, nil
}
