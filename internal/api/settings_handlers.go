package api

import (
	"net/http"

	"github.com/pborges/aiinventory/internal/gemini"
	"github.com/pborges/aiinventory/internal/store"
)

// promptSettingKeys maps each Gemini request type to its settings-table key.
var promptSettingKeys = map[gemini.RequestType]string{
	gemini.TagCapture:              store.SettingPromptTagCapture,
	gemini.LocationReconciliation:  store.SettingPromptLocationReconciliation,
	gemini.DescriptionRegeneration: store.SettingPromptDescriptionRegeneration,
	gemini.DuplicateDetection:      store.SettingPromptDuplicateDetection,
}

type promptSetting struct {
	Override string `json:"override"`
	Default  string `json:"default"`
}

type settingsResponse struct {
	GeminiModel        string                   `json:"gemini_model"`
	GeminiModelDefault string                   `json:"gemini_model_default"`
	Prompts            map[string]promptSetting `json:"prompts"`
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	resp, err := s.buildSettingsResponse(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) buildSettingsResponse(r *http.Request) (settingsResponse, error) {
	ctx := r.Context()

	model, _, err := s.store.GetSetting(ctx, store.SettingGeminiModel)
	if err != nil {
		return settingsResponse{}, err
	}

	prompts := make(map[string]promptSetting, len(promptSettingKeys))
	for rt, key := range promptSettingKeys {
		override, _, err := s.store.GetSetting(ctx, key)
		if err != nil {
			return settingsResponse{}, err
		}
		prompts[string(rt)] = promptSetting{Override: override, Default: gemini.DefaultPrompt(rt)}
	}

	return settingsResponse{
		GeminiModel:        model,
		GeminiModelDefault: gemini.DefaultModel,
		Prompts:            prompts,
	}, nil
}

type updateSettingsRequest struct {
	GeminiModel *string           `json:"gemini_model"`
	Prompts     map[string]string `json:"prompts"`
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req updateSettingsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()

	if req.GeminiModel != nil {
		if err := s.store.SetSetting(ctx, store.SettingGeminiModel, *req.GeminiModel); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	for rt, key := range promptSettingKeys {
		if v, ok := req.Prompts[string(rt)]; ok {
			if err := s.store.SetSetting(ctx, key, v); err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
		}
	}

	s.handleGetSettings(w, r)
}
