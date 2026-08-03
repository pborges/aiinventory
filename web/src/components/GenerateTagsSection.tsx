import { useEffect, useRef, useState } from 'preact/hooks'
import { ApiError, type TagSheetCutSettings, type TagSheetResponse, type UploadRegisteredTagsResponse } from '../api/client'

interface Props {
  title: string
  generate: (rows: number, cols: number, paddingMm: number, cutSettings: TagSheetCutSettings, codes?: string[]) => Promise<TagSheetResponse>
  register: (codes: string[]) => Promise<UploadRegisteredTagsResponse>
  fileBaseName: string
  onRegistered: () => void
}

const DEBOUNCE_MS = 400

function downloadText(filename: string, content: string, mime: string) {
  const url = URL.createObjectURL(new Blob([content], { type: mime }))
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

/** Generates a laser-cuttable sheet of pre-registered tags: a live preview
 * (debounced as the grid/padding change), per-operation LightBurn speed/
 * power/air-assist fields (defaulted for a 20W diode laser on 3mm
 * basswood, re-rendered into the same previewed codes when tweaked), a
 * checkbox per export format plus a "Register Tags" checkbox, and a single
 * Download button that does whichever of those are checked — the
 * one-click replacement for the old manual FreeCAD/LightBurn workflow.
 * Settings uses this for both the asset-tag and location-tag panes, which
 * differ only in which api.* methods and geometry get passed in. */
// DEFAULT_ROWS/COLS/PADDING_MM: the largest 60x26mm grid (at a
// laser-safe ~4mm kerf gap) that fits an 8.5x11in sheet in landscape —
// 4 cols x 60mm + 3 gaps = 252mm of 279.4mm (11in), 7 rows x 26mm + 6 gaps
// = 206mm of 215.9mm (8.5in). 8 rows would overflow the 8.5in dimension
// (236mm > 215.9mm) at this padding.
const DEFAULT_COLS = 4
const DEFAULT_ROWS = 7
const DEFAULT_PADDING_MM = 4

export function GenerateTagsSection({ title, generate, register, fileBaseName, onRegistered }: Props) {
  const [rows, setRows] = useState(DEFAULT_ROWS)
  const [cols, setCols] = useState(DEFAULT_COLS)
  const [padding, setPadding] = useState(DEFAULT_PADDING_MM)

  const [rasterSpeed, setRasterSpeed] = useState(3000)
  const [rasterPower, setRasterPower] = useState(40)
  const [rasterAirAssist, setRasterAirAssist] = useState(false)
  const [outlineSpeed, setOutlineSpeed] = useState(1500)
  const [outlinePower, setOutlinePower] = useState(25)
  const [outlineAirAssist, setOutlineAirAssist] = useState(false)
  const [cutSpeed, setCutSpeed] = useState(200)
  const [cutPower, setCutPower] = useState(100)
  const [cutAirAssist, setCutAirAssist] = useState(true)

  const [sheet, setSheet] = useState<TagSheetResponse | null>(null)
  const [previewUrl, setPreviewUrl] = useState<string | null>(null)
  const [downloadSvg, setDownloadSvg] = useState(false)
  const [downloadLbrn2, setDownloadLbrn2] = useState(true)
  const [registerTags, setRegisterTags] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [status, setStatus] = useState<string | null>(null)

  function cutSettingsPayload(): TagSheetCutSettings {
    return {
      raster_speed_mm_min: rasterSpeed,
      raster_power_pct: rasterPower,
      raster_air_assist: rasterAirAssist,
      outline_speed_mm_min: outlineSpeed,
      outline_power_pct: outlinePower,
      outline_air_assist: outlineAirAssist,
      cut_speed_mm_min: cutSpeed,
      cut_power_pct: cutPower,
      cut_air_assist: cutAirAssist,
    }
  }

  async function regenerate(opts?: { reuseCodes?: boolean }) {
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      const codes = opts?.reuseCodes ? sheet?.codes : undefined
      const resp = await generate(rows, cols, padding, cutSettingsPayload(), codes)
      setSheet(resp)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to generate preview')
    } finally {
      setBusy(false)
    }
  }

  useEffect(() => {
    const timer = setTimeout(() => regenerate(), DEBOUNCE_MS)
    return () => clearTimeout(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rows, cols, padding])

  // Skip the very first run (the effect above already generates an initial
  // preview on mount) — only re-render on a cut-settings change made after
  // that, and reuse the already-previewed codes rather than rerolling them.
  const skippedInitialCutSettingsRender = useRef(false)
  useEffect(() => {
    if (!skippedInitialCutSettingsRender.current) {
      skippedInitialCutSettingsRender.current = true
      return
    }
    const timer = setTimeout(() => regenerate({ reuseCodes: true }), DEBOUNCE_MS)
    return () => clearTimeout(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rasterSpeed, rasterPower, rasterAirAssist, outlineSpeed, outlinePower, outlineAirAssist, cutSpeed, cutPower, cutAirAssist])

  useEffect(() => {
    if (!sheet) {
      setPreviewUrl(null)
      return
    }
    const url = URL.createObjectURL(new Blob([sheet.svg], { type: 'image/svg+xml' }))
    setPreviewUrl(url)
    return () => URL.revokeObjectURL(url)
  }, [sheet])

  async function onDownload() {
    if (!sheet) return
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      let statusMsg = ''
      if (registerTags) {
        const res = await register(sheet.codes)
        statusMsg = `Registered ${res.added} tags (${res.skipped} skipped). `
        onRegistered()
      }

      const dateStamp = new Date().toISOString().slice(0, 10)
      const downloaded: string[] = []
      if (downloadSvg) {
        downloadText(`${fileBaseName}-${dateStamp}.svg`, sheet.svg, 'image/svg+xml')
        downloaded.push('SVG')
      }
      if (downloadLbrn2) {
        downloadText(`${fileBaseName}-${dateStamp}.lbrn2`, sheet.lbrn2, 'application/xml')
        downloaded.push('LBRN2')
      }
      if (downloaded.length) statusMsg += `Downloaded ${downloaded.join(' + ')}.`
      setStatus(statusMsg.trim())

      // Once registered, the previewed codes are no longer available —
      // reroll so a stale set can't be re-registered by clicking again.
      if (registerTags) regenerate()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to complete the requested action')
    } finally {
      setBusy(false)
    }
  }

  return (
    <section class="settings-registry">
      <h2>{title}</h2>

      <div class="generate-tags-form">
        <label>
          Columns
          <input
            class="generate-tags-field"
            type="number"
            min={1}
            max={20}
            value={cols}
            onInput={(e) => setCols(Number((e.target as HTMLInputElement).value) || 1)}
          />
        </label>
        <label>
          Rows
          <input
            class="generate-tags-field"
            type="number"
            min={1}
            max={30}
            value={rows}
            onInput={(e) => setRows(Number((e.target as HTMLInputElement).value) || 1)}
          />
        </label>
        <label class="generate-tags-unit">
          Padding
          <input
            class="generate-tags-field"
            type="number"
            min={0}
            max={50}
            value={padding}
            onInput={(e) => setPadding(Number((e.target as HTMLInputElement).value) || 0)}
          />
          <span>mm</span>
        </label>
        <button type="button" onClick={() => regenerate()} disabled={busy}>
          New Codes
        </button>
      </div>

      <div class="generate-tags-cutsettings">
        <div class="generate-tags-cutsettings-row">
          <span class="generate-tags-cutsettings-label">Raster Text</span>
          <label>
            Speed
            <input
              class="generate-tags-field"
              type="number"
              min={1}
              value={rasterSpeed}
              onInput={(e) => setRasterSpeed(Number((e.target as HTMLInputElement).value) || 1)}
            />
            <span>mm/min</span>
          </label>
          <label>
            Power
            <input
              class="generate-tags-field"
              type="number"
              min={1}
              max={100}
              value={rasterPower}
              onInput={(e) => setRasterPower(Number((e.target as HTMLInputElement).value) || 1)}
            />
            <span>%</span>
          </label>
          <label class="generate-tags-checkbox">
            <input
              type="checkbox"
              checked={rasterAirAssist}
              onChange={(e) => setRasterAirAssist((e.target as HTMLInputElement).checked)}
            />
            Air Assist
          </label>
        </div>

        <div class="generate-tags-cutsettings-row">
          <span class="generate-tags-cutsettings-label">Outline Text</span>
          <label>
            Speed
            <input
              class="generate-tags-field"
              type="number"
              min={1}
              value={outlineSpeed}
              onInput={(e) => setOutlineSpeed(Number((e.target as HTMLInputElement).value) || 1)}
            />
            <span>mm/min</span>
          </label>
          <label>
            Power
            <input
              class="generate-tags-field"
              type="number"
              min={1}
              max={100}
              value={outlinePower}
              onInput={(e) => setOutlinePower(Number((e.target as HTMLInputElement).value) || 1)}
            />
            <span>%</span>
          </label>
          <label class="generate-tags-checkbox">
            <input
              type="checkbox"
              checked={outlineAirAssist}
              onChange={(e) => setOutlineAirAssist((e.target as HTMLInputElement).checked)}
            />
            Air Assist
          </label>
        </div>

        <div class="generate-tags-cutsettings-row">
          <span class="generate-tags-cutsettings-label">Cut Tag</span>
          <label>
            Speed
            <input
              class="generate-tags-field"
              type="number"
              min={1}
              value={cutSpeed}
              onInput={(e) => setCutSpeed(Number((e.target as HTMLInputElement).value) || 1)}
            />
            <span>mm/min</span>
          </label>
          <label>
            Power
            <input
              class="generate-tags-field"
              type="number"
              min={1}
              max={100}
              value={cutPower}
              onInput={(e) => setCutPower(Number((e.target as HTMLInputElement).value) || 1)}
            />
            <span>%</span>
          </label>
          <label class="generate-tags-checkbox">
            <input type="checkbox" checked={cutAirAssist} onChange={(e) => setCutAirAssist((e.target as HTMLInputElement).checked)} />
            Air Assist
          </label>
        </div>
      </div>

      <div class="generate-tags-preview">
        {previewUrl ? <img src={previewUrl} alt={`${title} preview`} /> : <p>Generating preview…</p>}
      </div>

      <div class="generate-tags-actions">
        <label class="generate-tags-checkbox">
          <input type="checkbox" checked={downloadSvg} onChange={(e) => setDownloadSvg((e.target as HTMLInputElement).checked)} />
          SVG
        </label>
        <label class="generate-tags-checkbox">
          <input
            type="checkbox"
            checked={downloadLbrn2}
            onChange={(e) => setDownloadLbrn2((e.target as HTMLInputElement).checked)}
          />
          LBRN2
        </label>
        <label class="generate-tags-checkbox">
          <input
            type="checkbox"
            checked={registerTags}
            onChange={(e) => setRegisterTags((e.target as HTMLInputElement).checked)}
          />
          Register Tags
        </label>
        <button
          type="button"
          class="btn-primary"
          onClick={onDownload}
          disabled={busy || !sheet || (!downloadSvg && !downloadLbrn2 && !registerTags)}
        >
          Download
        </button>
      </div>
      <p class="settings-status">
        Speeds/powers default for a 20W diode laser on 3mm basswood — tune per material/machine in LightBurn. Register
        Tags commits the previewed codes to the registry above so they won't be generated again. Downloading both file
        formats may trigger Chrome's one-time multiple-downloads prompt.
      </p>
      {status && <p class="settings-status">{status}</p>}
      {error && <p class="settings-status settings-status-error">{error}</p>}
    </section>
  )
}
