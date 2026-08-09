package api

import (
	"log"
	"net/http"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/store"
)

// user_settings keys for the tag-sheet generator's persisted, per-user
// rows/cols/padding/cut-settings — asset and location tags are scoped
// separately since an operator may want different grids for each.
const (
	userSettingKeyAssetTagSheet    = "tag_sheet.asset"
	userSettingKeyLocationTagSheet = "tag_sheet.location"
)

// tagSheetSettingsRequest is both the persisted shape and the GET/PUT wire
// format for a tag sheet's rows/cols/padding/cut-settings — the same fields
// tagSheetRequest carries minus codes, since settings are geometry/cut
// config only, never a specific code batch.
type tagSheetSettingsRequest struct {
	Rows        int                        `json:"rows"`
	Cols        int                        `json:"cols"`
	PaddingMm   float64                    `json:"padding_mm"`
	CutSettings tagSheetCutSettingsRequest `json:"cut_settings"`
}

func (r tagSheetSettingsRequest) valid() bool {
	if r.Rows < 1 || r.Rows > tagSheetMaxRows {
		return false
	}
	if r.Cols < 1 || r.Cols > tagSheetMaxCols {
		return false
	}
	if r.PaddingMm < 0 || r.PaddingMm > tagSheetMaxPaddingMm {
		return false
	}
	return r.CutSettings.valid()
}

// defaultTagSheetSettings is the fallback for any user with no saved
// override — unchanged from what the webui hardcoded before this feature
// existed, so nobody's experience changes until they actually customize
// something. Asset and location tags share it (only the persisted override
// key differs), matching the single default the webui always used for
// both.
var defaultTagSheetSettings = tagSheetSettingsRequest{
	Rows: 6, Cols: 4, PaddingMm: 2,
	CutSettings: tagSheetCutSettingsRequest{
		RasterSpeedMmMin: 3500, RasterPowerPct: 17.5, RasterAirAssist: false,
		OutlineSpeedMmMin: 1500, OutlinePowerPct: 5, OutlineAirAssist: false,
		CutSpeedMmMin: 600, CutPowerPct: 100, CutAirAssist: true,
	},
}

func (s *Server) handleGetAssetTagSheetSettings(w http.ResponseWriter, r *http.Request) {
	s.getTagSheetSettings(w, r, userSettingKeyAssetTagSheet)
}

func (s *Server) handleSaveAssetTagSheetSettings(w http.ResponseWriter, r *http.Request) {
	s.saveTagSheetSettings(w, r, userSettingKeyAssetTagSheet)
}

func (s *Server) handleResetAssetTagSheetSettings(w http.ResponseWriter, r *http.Request) {
	s.resetTagSheetSettings(w, r, userSettingKeyAssetTagSheet)
}

func (s *Server) handleGetLocationTagSheetSettings(w http.ResponseWriter, r *http.Request) {
	s.getTagSheetSettings(w, r, userSettingKeyLocationTagSheet)
}

func (s *Server) handleSaveLocationTagSheetSettings(w http.ResponseWriter, r *http.Request) {
	s.saveTagSheetSettings(w, r, userSettingKeyLocationTagSheet)
}

func (s *Server) handleResetLocationTagSheetSettings(w http.ResponseWriter, r *http.Request) {
	s.resetTagSheetSettings(w, r, userSettingKeyLocationTagSheet)
}

// getTagSheetSettings returns the logged-in user's saved rows/cols/padding/
// cut-settings for key, falling back to defaultTagSheetSettings if they've
// never saved one.
func (s *Server) getTagSheetSettings(w http.ResponseWriter, r *http.Request, key string) {
	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	settings, err := store.GetUserSettingJSON(r.Context(), s.store, user.ID, key, defaultTagSheetSettings)
	if err != nil {
		log.Printf("tag sheet settings: get: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) saveTagSheetSettings(w http.ResponseWriter, r *http.Request, key string) {
	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req tagSheetSettingsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !req.valid() {
		writeError(w, http.StatusBadRequest, "invalid tag sheet settings")
		return
	}
	if err := store.SetUserSettingJSON(r.Context(), s.store, user.ID, key, req); err != nil {
		log.Printf("tag sheet settings: save: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, req)
}

// resetTagSheetSettings deletes the user's override for key ("restore
// defaults"), returning defaultTagSheetSettings — the same value a
// subsequent GET would now return.
func (s *Server) resetTagSheetSettings(w http.ResponseWriter, r *http.Request, key string) {
	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := s.store.DeleteUserSetting(r.Context(), user.ID, key); err != nil {
		log.Printf("tag sheet settings: reset: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, defaultTagSheetSettings)
}
