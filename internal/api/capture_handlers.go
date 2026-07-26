package api

import (
	"io"
	"net/http"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/gemini"
	"github.com/pborges/aiinventory/internal/inventory"
)

const maxUploadBytes = 10 << 20 // 10MB — captured frames are downsized client-side well below this

type captureResponse struct {
	HasAssetTag      bool   `json:"has_asset_tag"`
	AssetTag         string `json:"asset_tag,omitempty"`
	ItemID           int64  `json:"item_id,omitempty"`
	ItemWasNew       bool   `json:"item_was_new,omitempty"`
	ItemGuess        string `json:"item_guess,omitempty"`
	ImageDescription string `json:"image_description,omitempty"`
}

// handleCapture implements README flow #1: a captured photo is analyzed by
// Gemini for an asset tag; if found, the item is created or appended to.
func (s *Server) handleCapture(w http.ResponseWriter, r *http.Request) {
	if s.gemini == nil {
		writeError(w, http.StatusServiceUnavailable, "AI features are disabled (GEMINI_API_KEY not configured)")
		return
	}
	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing image file")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxUploadBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read image")
		return
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	ctx := r.Context()
	model, prompt, err := s.resolveGeminiConfig(ctx, gemini.TagCapture)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	analysis, err := s.gemini.AnalyzeTagCapture(ctx, model, prompt, data, contentType)
	if err != nil {
		writeError(w, http.StatusBadGateway, "gemini request failed: "+err.Error())
		return
	}

	if !analysis.HasAssetTag {
		writeJSON(w, http.StatusOK, captureResponse{HasAssetTag: false})
		return
	}

	result, err := inventory.Capture(ctx, s.store, user.ID, analysis.HasAssetTag, analysis.AssetTag, data, contentType, analysis.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, captureResponse{
		HasAssetTag:      true,
		AssetTag:         result.Item.AssetTag,
		ItemID:           result.Item.ID,
		ItemWasNew:       result.ItemWasNew,
		ItemGuess:        analysis.ItemGuess,
		ImageDescription: analysis.Description,
	})
}
