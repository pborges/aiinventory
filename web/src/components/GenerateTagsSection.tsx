import { useEffect, useRef, useState } from 'preact/hooks'
import {
  ApiError,
  type TagSheetCutSettings,
  type TagSheetResponse,
  type TagSheetSettings,
  type UploadRegisteredTagsResponse,
} from '../api/client'

interface Props {
  title: string
  generate: (rows: number, cols: number, paddingMm: number, cutSettings: TagSheetCutSettings, codes?: string[]) => Promise<TagSheetResponse>
  register: (codes: string[]) => Promise<UploadRegisteredTagsResponse>
  getSettings: () => Promise<TagSheetSettings>
  saveSettings: (settings: TagSheetSettings) => Promise<TagSheetSettings>
  resetSettings: () => Promise<TagSheetSettings>
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

// downloadBase64 decodes a base64-encoded binary payload (the .ryp project
// file is a zip archive, not text) into a Blob and downloads it the same
// way downloadText does for the SVG/LBRN2 formats.
function downloadBase64(filename: string, base64: string, mime: string) {
  const binary = atob(base64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  const url = URL.createObjectURL(new Blob([bytes], { type: mime }))
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

type DownloadFormat = 'svg' | 'lbrn2' | 'rayforge'

/** Generates a laser-cuttable sheet of pre-registered tags: a live preview
 * (debounced as the grid/padding change), per-operation LightBurn speed/
 * power/air-assist fields — plus a line-spacing field for the raster pass
 * only, vector cuts having no scan lines — (defaulted for a 20W diode
 * laser on 3mm basswood, re-rendered into the same previewed codes when
 * tweaked), a
 * per-code checkbox grid (pre-checked) so codes that didn't cut well can
 * be excluded, a "Load Codes" file picker that renders an operator-supplied
 * newline-separated .txt of codes instead of a random draw — one tag per
 * code, up to rows*cols, with any leftover grid cells left blank rather
 * than padded out — a Download button gated by an SVG/LBRN2/Rayforge format
 * radio group plus a "Codes" checkbox that, alongside the sheet file,
 * downloads a same-GUID-named .txt of the codes on it (both files are
 * named from a fresh crypto.randomUUID() per download so a sheet and its
 * codes list can be paired up later), and a separate Register button that
 * commits only the checked codes — the
 * intended flow is download, cut, uncheck any that failed, then register.
 * Settings uses this for both the asset-tag and location-tag panes, which
 * differ only in which api.* methods and geometry get passed in. Rows/
 * cols/padding/cut-settings are per-user persisted server-side (getSettings/
 * saveSettings/resetSettings): loaded on mount in place of the hardcoded
 * literals below, written back only on an explicit Save click (no
 * autosave-on-change/tab-out — a mid-tweak value shouldn't silently
 * become the new persisted default), and reset via "Restore Defaults" —
 * which restores exactly those hardcoded literals, since that's the
 * server's own fallback for a user with no saved override. */
// DEFAULT_ROWS/COLS/PADDING_MM: a 60x26mm grid (at a laser-safe ~2mm
// kerf gap) that fits an 8.5x11in sheet in landscape — 4 cols x 60mm +
// 3 gaps = 246mm of 279.4mm (11in), 6 rows x 26mm + 5 gaps = 166mm of
// 215.9mm (8.5in).
const DEFAULT_COLS = 4
const DEFAULT_ROWS = 6
const DEFAULT_PADDING_MM = 2

export function GenerateTagsSection({ title, generate, register, getSettings, saveSettings, resetSettings, fileBaseName, onRegistered }: Props) {
  const [rows, setRows] = useState(DEFAULT_ROWS)
  const [cols, setCols] = useState(DEFAULT_COLS)
  const [padding, setPadding] = useState(DEFAULT_PADDING_MM)

  const [rasterSpeed, setRasterSpeed] = useState(7000)
  const [rasterPower, setRasterPower] = useState(32)
  const [rasterAirAssist, setRasterAirAssist] = useState(false)
  const [rasterLineInterval, setRasterLineInterval] = useState(0.05)
  const [cutSpeed, setCutSpeed] = useState(600)
  const [cutPower, setCutPower] = useState(100)
  const [cutAirAssist, setCutAirAssist] = useState(true)

  const fileInputRef = useRef<HTMLInputElement>(null)

  const [sheet, setSheet] = useState<TagSheetResponse | null>(null)
  const [previewUrl, setPreviewUrl] = useState<string | null>(null)
  const [downloadFormat, setDownloadFormat] = useState<DownloadFormat>('rayforge')
  const [downloadCodesList, setDownloadCodesList] = useState(true)
  const [checkedCodes, setCheckedCodes] = useState<Set<string>>(new Set())
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [status, setStatus] = useState<string | null>(null)

  // Tracks whether the persisted settings fetch (below) has resolved, so
  // Save/Restore Defaults can't fire — and potentially clobber a real
  // saved config with the hardcoded placeholder state — before it's known
  // whether that placeholder state is even still current.
  const [settingsLoaded, setSettingsLoaded] = useState(false)

  function settingsPayload(): TagSheetSettings {
    return { rows, cols, padding_mm: padding, cut_settings: cutSettingsPayload() }
  }

  function applySettings(s: TagSheetSettings) {
    setRows(s.rows)
    setCols(s.cols)
    setPadding(s.padding_mm)
    setRasterSpeed(s.cut_settings.raster_speed_mm_min)
    setRasterPower(s.cut_settings.raster_power_pct)
    setRasterAirAssist(s.cut_settings.raster_air_assist)
    setRasterLineInterval(s.cut_settings.raster_line_interval_mm)
    setCutSpeed(s.cut_settings.cut_speed_mm_min)
    setCutPower(s.cut_settings.cut_power_pct)
    setCutAirAssist(s.cut_settings.cut_air_assist)
  }

  // Loads this user's saved rows/cols/padding/cut-settings once on mount,
  // replacing the hardcoded placeholder state above — falls back to
  // whatever's already there (the same hardcoded defaults the server would
  // return anyway for a user with no override) if the fetch fails.
  useEffect(() => {
    let cancelled = false
    getSettings()
      .then((s) => {
        if (cancelled) return
        applySettings(s)
        setSettingsLoaded(true)
      })
      .catch(() => setSettingsLoaded(true))
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function onSaveSettings() {
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      await saveSettings(settingsPayload())
      setStatus('Settings saved.')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to save settings')
    } finally {
      setBusy(false)
    }
  }

  async function onRestoreDefaults() {
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      const defaults = await resetSettings()
      applySettings(defaults)
      setStatus('Restored default settings.')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to restore defaults')
    } finally {
      setBusy(false)
    }
  }

  function toggleCode(code: string, checked: boolean) {
    setCheckedCodes((prev) => {
      const next = new Set(prev)
      if (checked) next.add(code)
      else next.delete(code)
      return next
    })
  }

  function cutSettingsPayload(): TagSheetCutSettings {
    return {
      raster_speed_mm_min: rasterSpeed,
      raster_power_pct: rasterPower,
      raster_air_assist: rasterAirAssist,
      raster_line_interval_mm: rasterLineInterval,
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

  // Renders exactly the codes in the chosen .txt (one label per line,
  // blank lines ignored) instead of a fresh random draw — one tag per
  // code, up to rows*cols; if the file has fewer codes than the grid
  // holds, the remaining cells are simply left blank rather than padded
  // out with random codes.
  async function onLoadCodes() {
    const file = fileInputRef.current?.files?.[0]
    if (!file) {
      setError('Choose a .txt file first')
      return
    }
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      const text = await file.text()
      const codes = text
        .split(/\r?\n/)
        .map((line) => line.trim())
        .filter(Boolean)
      if (!codes.length) {
        setError('That file has no codes in it')
        return
      }
      if (codes.length > rows * cols) {
        setError(`File has ${codes.length} codes, but the ${rows}×${cols} grid only holds ${rows * cols}. Increase rows/cols or trim the file.`)
        return
      }
      const resp = await generate(rows, cols, padding, cutSettingsPayload(), codes)
      setSheet(resp)
      setStatus(`Loaded ${resp.codes.length} code${resp.codes.length === 1 ? '' : 's'} from ${file.name}.`)
      if (fileInputRef.current) fileInputRef.current.value = ''
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load codes')
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
  }, [rasterSpeed, rasterPower, rasterAirAssist, rasterLineInterval, cutSpeed, cutPower, cutAirAssist])

  useEffect(() => {
    if (!sheet) {
      setPreviewUrl(null)
      return
    }
    const url = URL.createObjectURL(new Blob([sheet.svg], { type: 'image/svg+xml' }))
    setPreviewUrl(url)
    return () => URL.revokeObjectURL(url)
  }, [sheet])

  // A freshly (re)generated sheet's codes start all checked — the operator
  // unchecks the ones that didn't cut well before hitting Register.
  useEffect(() => {
    setCheckedCodes(new Set(sheet?.codes ?? []))
  }, [sheet])

  async function onDownload() {
    if (!sheet) return
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      const guid = crypto.randomUUID()
      let formatLabel: string
      switch (downloadFormat) {
        case 'svg':
          downloadText(`${guid}.svg`, sheet.svg, 'image/svg+xml')
          formatLabel = 'SVG'
          break
        case 'lbrn2':
          downloadText(`${guid}.lbrn2`, sheet.lbrn2, 'application/xml')
          formatLabel = 'LBRN2'
          break
        case 'rayforge':
          downloadBase64(`${guid}.ryp`, sheet.rayforge, 'application/zip')
          formatLabel = 'Rayforge project'
          break
      }
      if (downloadCodesList) {
        downloadText(`${guid}.txt`, sheet.codes.join('\n'), 'text/plain')
        setStatus(`Downloaded ${formatLabel} and codes list.`)
      } else {
        setStatus(`Downloaded ${formatLabel}.`)
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to download')
    } finally {
      setBusy(false)
    }
  }

  async function onRegister() {
    if (!sheet) return
    const codes = sheet.codes.filter((code) => checkedCodes.has(code))
    if (!codes.length) return
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      const res = await register(codes)
      setStatus(`Registered ${res.added} tags (${res.skipped} skipped).`)
      onRegistered()
      // Once registered, the previewed codes are no longer available —
      // reroll so a stale set can't be re-registered by clicking again.
      regenerate()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to register tags')
    } finally {
      setBusy(false)
    }
  }

  return (
    <section class="settings-registry">
      <h2>{title}</h2>

      <div class="generate-tags-cutsettings">
        <div class="generate-tags-cutsettings-row">
          <span class="generate-tags-cutsettings-label">Grid</span>
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
        </div>

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
          <label>
            Line Spacing
            <input
              class="generate-tags-field"
              type="number"
              min={0.01}
              step={0.01}
              value={rasterLineInterval}
              onInput={(e) => setRasterLineInterval(Number((e.target as HTMLInputElement).value) || 0.01)}
            />
            <span>mm</span>
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

        <div class="generate-tags-cutsettings-row">
          <span class="generate-tags-cutsettings-label">Codes</span>
          <input ref={fileInputRef} class="generate-tags-file-input" type="file" accept=".txt" disabled={busy} />
          <button type="button" onClick={onLoadCodes} disabled={busy}>
            Load Codes
          </button>
        </div>

        <div class="generate-tags-cutsettings-row generate-tags-cutsettings-actions">
          <button type="button" onClick={() => regenerate()} disabled={busy}>
            New Codes
          </button>
          <button type="button" onClick={onSaveSettings} disabled={busy || !settingsLoaded}>
            Save
          </button>
          <button type="button" onClick={onRestoreDefaults} disabled={busy || !settingsLoaded}>
            Restore Defaults
          </button>
        </div>
      </div>

      <div class="generate-tags-preview">
        {previewUrl ? <img src={previewUrl} alt={`${title} preview`} /> : <p>Generating preview…</p>}
      </div>

      {sheet && sheet.codes.length > 0 && (
        <div class="generate-tags-codes">
          {sheet.codes.map((code) => (
            <label class="generate-tags-checkbox" key={code}>
              <input
                type="checkbox"
                checked={checkedCodes.has(code)}
                onChange={(e) => toggleCode(code, (e.target as HTMLInputElement).checked)}
              />
              {code}
            </label>
          ))}
        </div>
      )}

      <div class="generate-tags-actions">
        <label class="generate-tags-checkbox">
          <input
            type="radio"
            name={`${fileBaseName}-download-format`}
            checked={downloadFormat === 'svg'}
            onChange={() => setDownloadFormat('svg')}
          />
          SVG
        </label>
        <label class="generate-tags-checkbox">
          <input
            type="radio"
            name={`${fileBaseName}-download-format`}
            checked={downloadFormat === 'lbrn2'}
            onChange={() => setDownloadFormat('lbrn2')}
          />
          LBRN2
        </label>
        <label class="generate-tags-checkbox">
          <input
            type="radio"
            name={`${fileBaseName}-download-format`}
            checked={downloadFormat === 'rayforge'}
            onChange={() => setDownloadFormat('rayforge')}
          />
          Rayforge
        </label>
        <label class="generate-tags-checkbox">
          <input
            type="checkbox"
            checked={downloadCodesList}
            onChange={(e) => setDownloadCodesList((e.target as HTMLInputElement).checked)}
          />
          Codes
        </label>
        <button type="button" class="btn-primary" onClick={onDownload} disabled={busy || !sheet}>
          Download
        </button>
        <button type="button" class="btn-primary" onClick={onRegister} disabled={busy || !sheet || checkedCodes.size === 0}>
          Register
        </button>
      </div>
      <p class="settings-status">
        Speeds/powers default for a 20W diode laser on 3mm basswood — tune per material/machine in LightBurn or
        Rayforge. Download, cut the sheet, uncheck any codes that didn't cut well, then Register to commit the rest
        to the registry above.
      </p>
      {status && <p class="settings-status">{status}</p>}
      {error && <p class="settings-status settings-status-error">{error}</p>}
    </section>
  )
}
