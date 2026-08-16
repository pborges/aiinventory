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
	UpdateItemDescriptionWithActivity(ctx context.Context, userID, itemID int64, description string, action domain.ActivityAction) error
}

// RegenerateDescription implements the Search view's bulk "Regenerate
// description" action (and the item detail view's single-item version):
// Gemini reviews every per-image note attached to the item and consolidates
// them into one description, explicitly preserving serial/part numbers (see
// the description_regeneration default prompt). hint is an optional
// user-supplied steer for this specific run; pass "" when there is none.
func RegenerateDescription(ctx context.Context, s DescriptionStore, g gemini.Client, userID int64, model, prompt string, itemID int64, hint string) (string, error) {
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

	result, err := g.RegenerateDescription(ctx, model, prompt, item.AssetTag, notes, hint)
	if err != nil {
		return "", fmt.Errorf("gemini: %w", err)
	}

	if err := s.UpdateItemDescriptionWithActivity(ctx, userID, itemID, result.Description, domain.ActivityDescriptionRegenerated); err != nil {
		return "", fmt.Errorf("update description: %w", err)
	}

	return result.Description, nil
}
