package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/pborges/aiinventory/internal/tagsheet"
)

const (
	tagSheetMaxRows           = 30
	tagSheetMaxCols           = 20
	tagSheetMaxPaddingMm      = 50
	tagSheetMaxCodes          = 600
	tagSheetMaxSpeedMmMin     = 100000
	tagSheetMaxLineIntervalMm = 5
)

type tagSheetCutSettingsRequest struct {
	RasterSpeedMmMin     float64 `json:"raster_speed_mm_min"`
	RasterPowerPct       float64 `json:"raster_power_pct"`
	RasterAirAssist      bool    `json:"raster_air_assist"`
	RasterLineIntervalMm float64 `json:"raster_line_interval_mm"`
	CutSpeedMmMin        float64 `json:"cut_speed_mm_min"`
	CutPowerPct          float64 `json:"cut_power_pct"`
	CutAirAssist         bool    `json:"cut_air_assist"`
}

func (r tagSheetCutSettingsRequest) valid() bool {
	for _, speed := range []float64{r.RasterSpeedMmMin, r.CutSpeedMmMin} {
		if speed <= 0 || speed > tagSheetMaxSpeedMmMin {
			return false
		}
	}
	for _, power := range []float64{r.RasterPowerPct, r.CutPowerPct} {
		if power <= 0 || power > 100 {
			return false
		}
	}
	if r.RasterLineIntervalMm <= 0 || r.RasterLineIntervalMm > tagSheetMaxLineIntervalMm {
		return false
	}
	return true
}

func (r tagSheetCutSettingsRequest) toTagsheet() tagsheet.CutSettings {
	return tagsheet.CutSettings{
		RasterSpeedMmMin: r.RasterSpeedMmMin, RasterPowerPct: r.RasterPowerPct, RasterAirAssist: r.RasterAirAssist, RasterLineIntervalMm: r.RasterLineIntervalMm,
		CutSpeedMmMin: r.CutSpeedMmMin, CutPowerPct: r.CutPowerPct, CutAirAssist: r.CutAirAssist,
	}
}

type tagSheetRequest struct {
	Rows      int     `json:"rows"`
	Cols      int     `json:"cols"`
	PaddingMm float64 `json:"padding_mm"`
	// Codes, if set, re-renders exactly this batch (already vetted by an
	// earlier call to this same endpoint) instead of drawing a fresh one —
	// how the UI updates the downloadable .lbrn2 after a cut-settings-only
	// tweak without rerolling the previewed codes out from under the
	// operator.
	Codes       []string                   `json:"codes,omitempty"`
	CutSettings tagSheetCutSettingsRequest `json:"cut_settings"`
}

type tagSheetResponse struct {
	Codes []string `json:"codes"`
	SVG   string   `json:"svg"`
	LBRN2 string   `json:"lbrn2"`
	// Rayforge is a base64-encoded .ryp project file (a zip archive) —
	// unlike SVG/LBRN2 this isn't text, so it can't go straight into a
	// JSON string field.
	Rayforge string `json:"rayforge"`
}

type tagSheetRegisterRequest struct {
	Codes []string `json:"codes"`
}

// handleGenerateAssetTagSheet and its three siblings below are thin
// wrappers around generateTagSheet/registerTagSheet, parameterized on
// what's different between the asset-tag and location-tag flows: the
// tagsheet.Kind (geometry/font), the code shape (letters/prefix), and
// which store methods supply the exclusion set / do the registration.
func (s *Server) handleGenerateAssetTagSheet(w http.ResponseWriter, r *http.Request) {
	s.generateTagSheet(w, r, tagsheet.KindAsset, 4, "", assetTagPattern, s.store.ListAllKnownAssetTags)
}

func (s *Server) handleRegisterAssetTagSheet(w http.ResponseWriter, r *http.Request) {
	s.registerTagSheet(w, r, assetTagPattern, s.store.BulkRegisterAssetTags)
}

func (s *Server) handleGenerateLocationTagSheet(w http.ResponseWriter, r *http.Request) {
	s.generateTagSheet(w, r, tagsheet.KindLocation, 3, "@", locationTagPattern, s.store.ListAllKnownLocationTags)
}

func (s *Server) handleRegisterLocationTagSheet(w http.ResponseWriter, r *http.Request) {
	s.registerTagSheet(w, r, locationTagPattern, s.store.BulkRegisterLocationTags)
}

// normalizeCodes uppercases/trims every code and reports any that don't
// match pattern — shared by generateTagSheet's codes-passthrough path and
// registerTagSheet, neither of which trusts a client-supplied batch as-is.
func normalizeCodes(codes []string, pattern *regexp.Regexp) (normalized, invalid []string) {
	normalized = make([]string, len(codes))
	for i, code := range codes {
		code = strings.ToUpper(strings.TrimSpace(code))
		normalized[i] = code
		if !pattern.MatchString(code) {
			invalid = append(invalid, code)
		}
	}
	return normalized, invalid
}

// generateTagSheet is the preview endpoint. With no req.Codes, it draws
// rows*cols fresh codes guaranteed not to collide with anything known(ctx)
// already returns (registered tags plus tags already in use by an
// item/location); with req.Codes set, it re-renders that exact
// already-vetted batch instead (a cut-settings-only tweak shouldn't reroll
// the codes an operator is already looking at). Either way it lays the
// result out and returns both export formats — nothing is persisted here,
// registration only happens when the operator actually downloads, via
// registerTagSheet.
func (s *Server) generateTagSheet(w http.ResponseWriter, r *http.Request, kind tagsheet.Kind, letters int, prefix string, pattern *regexp.Regexp, known func(context.Context) ([]string, error)) {
	var req tagSheetRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Rows < 1 || req.Rows > tagSheetMaxRows {
		writeError(w, http.StatusBadRequest, "rows must be between 1 and 30")
		return
	}
	if req.Cols < 1 || req.Cols > tagSheetMaxCols {
		writeError(w, http.StatusBadRequest, "cols must be between 1 and 20")
		return
	}
	if req.PaddingMm < 0 || req.PaddingMm > tagSheetMaxPaddingMm {
		writeError(w, http.StatusBadRequest, "padding_mm must be between 0 and 50")
		return
	}
	if !req.CutSettings.valid() {
		writeError(w, http.StatusBadRequest, "cut settings must have positive speeds and power percentages between 0 and 100")
		return
	}

	ctx := r.Context()

	var codes []string
	if len(req.Codes) > 0 {
		if len(req.Codes) != req.Rows*req.Cols {
			writeError(w, http.StatusBadRequest, "codes must contain exactly rows*cols entries")
			return
		}
		normalized, invalid := normalizeCodes(req.Codes, pattern)
		if len(invalid) > 0 {
			writeError(w, http.StatusBadRequest, "invalid codes: "+strings.Join(invalid, ", "))
			return
		}
		codes = normalized
	} else {
		knownTags, err := known(ctx)
		if err != nil {
			log.Printf("tag sheet: list known tags: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		exclude := make(map[string]struct{}, len(knownTags))
		for _, tag := range knownTags {
			exclude[tag] = struct{}{}
		}

		codes, err = tagsheet.GenerateCodes(rand.Reader, req.Rows*req.Cols, letters, prefix, exclude)
		if err != nil {
			log.Printf("tag sheet: generate codes: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	sheet, err := tagsheet.Layout(kind, codes, req.Rows, req.Cols, req.PaddingMm)
	if err != nil {
		log.Printf("tag sheet: layout: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rayforgeZip, err := tagsheet.RenderRayforge(sheet, req.CutSettings.toTagsheet())
	if err != nil {
		log.Printf("tag sheet: render rayforge: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, tagSheetResponse{
		Codes:    codes,
		SVG:      tagsheet.RenderSVG(sheet),
		LBRN2:    tagsheet.RenderLBRN2(sheet, req.CutSettings.toTagsheet()),
		Rayforge: base64.StdEncoding.EncodeToString(rayforgeZip),
	})
}

// registerTagSheet registers exactly the codes the operator's preview
// produced — the write side triggered by the Download button when its
// "Register Tags" checkbox is on. Codes are re-validated against pattern
// rather than trusted as-is: nothing stops a modified/replayed request
// from passing through generateTagSheet's shape guarantees.
func (s *Server) registerTagSheet(w http.ResponseWriter, r *http.Request, pattern *regexp.Regexp, bulkRegister func(context.Context, []string) (added, skipped int, err error)) {
	var req tagSheetRegisterRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Codes) < 1 || len(req.Codes) > tagSheetMaxCodes {
		writeError(w, http.StatusBadRequest, "codes must contain between 1 and 600 entries")
		return
	}

	normalized, invalid := normalizeCodes(req.Codes, pattern)
	if len(invalid) > 0 {
		writeError(w, http.StatusBadRequest, "invalid codes: "+strings.Join(invalid, ", "))
		return
	}

	added, skipped, err := bulkRegister(r.Context(), normalized)
	if err != nil {
		log.Printf("tag sheet: bulk register: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, registeredTagUploadResponse{Added: added, Skipped: skipped})
}
