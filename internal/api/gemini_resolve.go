package api

import (
	"context"

	"github.com/pborges/aiinventory/internal/gemini"
	"github.com/pborges/aiinventory/internal/store"
)

// resolveGeminiConfig returns the model and prompt to use for request type
// rt: the settings-table override if one is set, else the compiled-in
// default (see internal/gemini.DefaultPrompt / DefaultModel).
func (s *Server) resolveGeminiConfig(ctx context.Context, rt gemini.RequestType) (model, prompt string, err error) {
	model, ok, err := s.store.GetSetting(ctx, store.SettingGeminiModel)
	if err != nil {
		return "", "", err
	}
	if !ok || model == "" {
		model = gemini.DefaultModel
	}

	prompt = gemini.DefaultPrompt(rt)
	if key, isKnown := promptSettingKeys[rt]; isKnown {
		override, ok, err := s.store.GetSetting(ctx, key)
		if err != nil {
			return "", "", err
		}
		if ok && override != "" {
			prompt = override
		}
	}
	return model, prompt, nil
}
