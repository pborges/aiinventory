import { useEffect, useRef, useState } from 'preact/hooks'
import { faCamera, faMap, faXmark, faCheck } from '@fortawesome/free-solid-svg-icons'
import { api, ApiError, type ReconcileDiffResponse, type TagResolution } from '../api/client'
import { captureSquareFrame, rotateSquareBlob } from '../lib/camera'
import { ReconcileDiff } from '../components/ReconcileDiff'
import { TagAgreementReview, type TagReviewRow } from '../components/TagAgreementReview'
import { Header } from '../components/Header'
import { Footer } from '../components/Footer'
import { Icon } from '../components/Icon'

interface RouteProps {
  path?: string
  default?: boolean
}

type Phase = 'live' | 'analyzing' | 'awaiting-accept' | 'awaiting-location-tag' | 'committing' | 'result'
type CaptureMode = 'ingest' | 'locate'

interface PendingCapture {
  assetTag: string
  rawAssetTag: string
  corrected: boolean
  needsResolution: boolean
  candidates: string[]
  guess: string
  description: string
  itemWillBeNew: boolean
}

const ASSET_TAG_PATTERN = /^[A-Z]{4}$/
const LOCATION_TAG_PATTERN = /^@[A-Z]{3}$/

type ResultData = { kind: 'nothing' } | { kind: 'error'; message: string }

interface TagReviewState {
  locationTag: string
  agreedTags: string[]
  rows: TagReviewRow[]
}

// Stashes the locate flow's straight/rotated preview reads while the
// operator resolves an unconfident location-tag OCR read — so the flow can
// resume straight into the asset-tag presence-agreement step without
// re-calling Gemini.
interface LocationTagResolutionState {
  straight: ReconcileDiffResponse
  rotated: ReconcileDiffResponse
}

// Builds the "needs a human look" row set for one raw asset tag, from
// whichever read(s) flagged it — used for both the tag-review step and the
// corrected-tag annotations carried into the final approval modal.
function resolutionsByRaw(resolutionLists: (TagResolution[] | undefined)[]): Map<string, TagResolution> {
  const byRaw = new Map<string, TagResolution>()
  for (const list of resolutionLists) {
    for (const res of list ?? []) {
      const existing = byRaw.get(res.raw)
      if (!existing || (existing.status === 'exact' && res.status !== 'exact')) {
        byRaw.set(res.raw, res)
      }
    }
  }
  return byRaw
}

export function Capture(_props: RouteProps) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const capturedBlobRef = useRef<Blob | null>(null)
  const [cameraError, setCameraError] = useState<string | null>(null)
  const [phase, setPhase] = useState<Phase>('live')
  const [mode, setMode] = useState<CaptureMode>('ingest')
  const [frozenFrameUrl, setFrozenFrameUrl] = useState<string | null>(null)
  const [pendingCapture, setPendingCapture] = useState<PendingCapture | null>(null)
  const [resolvedTag, setResolvedTag] = useState('')
  const [result, setResult] = useState<ResultData | null>(null)
  const [pendingDiff, setPendingDiff] = useState<ReconcileDiffResponse | null>(null)
  const [applyingReconcile, setApplyingReconcile] = useState(false)
  const [tagReview, setTagReview] = useState<TagReviewState | null>(null)
  const [confirmingTagReview, setConfirmingTagReview] = useState(false)
  const [locationTagResolution, setLocationTagResolution] = useState<LocationTagResolutionState | null>(null)
  const [resolvedLocationTag, setResolvedLocationTag] = useState('')
  // Whether to carry the AI-read description over onto the saved photo —
  // checked by default the moment a preview with a description comes back,
  // since it's usually right; unchecking it saves the photo with no notes.
  const [acceptDescription, setAcceptDescription] = useState(true)
  // resolved tag -> raw OCR read, for any tag that reached the approval
  // modal via a registry correction — carried separately from pendingDiff
  // since /api/reconcile/diff doesn't re-run resolution and can't
  // reconstruct this after the tag-review step.
  const [correctedTags, setCorrectedTags] = useState<Record<string, string>>({})
  // Settings → Gemini configuration's "Dual-read location tag cross-check"
  // toggle — defaults true (the flow's original always-on behavior) until
  // the real value loads, so a slow settings fetch never silently disables it.
  const [dualReadEnabled, setDualReadEnabled] = useState(true)

  useEffect(() => {
    api
      .getSettings()
      .then((s) => setDualReadEnabled(s.location_dual_read_enabled))
      .catch(() => {})
  }, [])

  useEffect(() => {
    let stream: MediaStream | null = null
    let cancelled = false

    navigator.mediaDevices
      .getUserMedia({ video: { facingMode: 'environment' }, audio: false })
      .then((s) => {
        if (cancelled) {
          s.getTracks().forEach((t) => t.stop())
          return
        }
        stream = s
        if (videoRef.current) {
          videoRef.current.srcObject = s
        }
      })
      .catch((err) => setCameraError(err instanceof Error ? err.message : 'Could not access camera'))

    return () => {
      cancelled = true
      stream?.getTracks().forEach((t) => t.stop())
    }
  }, [])

  useEffect(() => {
    // revoke the frozen-frame object URL whenever it's replaced or on unmount
    return () => {
      if (frozenFrameUrl) URL.revokeObjectURL(frozenFrameUrl)
    }
  }, [frozenFrameUrl])

  function resetToLive() {
    setFrozenFrameUrl(null)
    setPendingCapture(null)
    setResolvedTag('')
    setResult(null)
    setCorrectedTags({})
    setLocationTagResolution(null)
    setResolvedLocationTag('')
    setAcceptDescription(true)
    capturedBlobRef.current = null
    setPhase('live')
  }

  // Runs the asset-tag presence-agreement step against an already-confirmed
  // location tag — shared by both the exact-match path (called straight out
  // of onCapture) and the post-resolver path (called from
  // onConfirmLocationTag). If the confirmed tag differs from what the
  // straight/rotated diffs were actually computed against (only possible
  // when a resolution happened), the diff is stale and gets refetched
  // before anything is shown for approval.
  async function continueWithAssetTags(locationTag: string, straight: ReconcileDiffResponse, rotated: ReconcileDiffResponse) {
    // Presence agreement is computed over the RAW reads (every tag_
    // resolutions entry, not just the ones that made it into asset_tags —
    // asset_tags on a preview response only ever contains exact registry
    // matches; anything corrected/ambiguous/unmatched is deliberately
    // held back from it, see handleReconcilePreview).
    const straightRaw = (straight.tag_resolutions ?? []).map((r) => r.raw)
    const rotatedRaw = (rotated.tag_resolutions ?? []).map((r) => r.raw)
    const straightRawSet = new Set(straightRaw)
    const rotatedRawSet = new Set(rotatedRaw)
    const presenceAgreed = straightRaw.filter((tag) => rotatedRawSet.has(tag))
    const presenceDiff = [...new Set([...straightRaw, ...rotatedRaw])].filter(
      (tag) => !(straightRawSet.has(tag) && rotatedRawSet.has(tag)),
    )

    const resByRaw = resolutionsByRaw([straight.tag_resolutions, rotated.tag_resolutions])

    // Trusted enough to skip review entirely: both reads saw it AND it's
    // an exact registry match.
    const agreedTags = presenceAgreed.filter((tag) => resByRaw.get(tag)?.status === 'exact')

    // Everything else needs a human look: presence disagreements, or any
    // tag (agreed-on or not) that isn't an exact registry match.
    const needsReview = new Set(presenceDiff)
    for (const [raw, res] of resByRaw) {
      if (res.status !== 'exact') needsReview.add(raw)
    }
    for (const tag of agreedTags) needsReview.delete(tag)

    if (needsReview.size === 0) {
      try {
        // stays in 'analyzing' (spinner) until the modal is resolved
        if (locationTag === straight.location_tag) {
          setPendingDiff(straight) // full agreement, confirmed tag matches what the diff was computed against
        } else {
          setPendingDiff(await api.reconcileDiff(locationTag, agreedTags))
        }
      } catch (err) {
        setResult({ kind: 'error', message: err instanceof ApiError ? err.message : 'Capture failed' })
        setPhase('result')
      }
      return
    }

    // A confident single-candidate correction is still worth surfacing on
    // the final approval screen even after the operator confirms it here.
    const corrections: Record<string, string> = {}
    for (const [raw, res] of resByRaw) {
      if (res.status === 'corrected' && res.candidates?.[0]) corrections[res.candidates[0]] = raw
    }
    setCorrectedTags(corrections)

    const rows: TagReviewRow[] = [...needsReview].map((raw) => ({
      raw,
      candidates: resByRaw.get(raw)?.candidates ?? [],
    }))
    setTagReview({ locationTag, agreedTags, rows })
  }

  async function onCapture() {
    if (!videoRef.current || phase !== 'live') return

    const blob = await captureSquareFrame(videoRef.current)
    capturedBlobRef.current = blob
    setFrozenFrameUrl(URL.createObjectURL(blob))
    setResult(null) // clear the previous cycle's confirmation once a new one starts
    setPhase('analyzing')

    try {
      // Which flow runs is an explicit user choice (the mode toggle below
      // the viewfinder), not auto-detected — asset tags and location tags
      // can both be a handful of uppercase letters on a white sticker, and
      // guessing which one Gemini should look for was unreliable in
      // practice. Analyzing a tag never writes anything by itself; the
      // user must Accept.
      if (mode === 'ingest') {
        const preview = await api.capturePreview(blob)
        if (preview.has_asset_tag) {
          const needsResolution = !!preview.needs_resolution
          setPendingCapture({
            assetTag: preview.asset_tag ?? '',
            rawAssetTag: preview.raw_asset_tag ?? '',
            corrected: !!preview.corrected,
            needsResolution,
            candidates: preview.candidates ?? [],
            guess: preview.item_guess ?? '',
            description: preview.image_description ?? '',
            itemWillBeNew: !!preview.item_will_be_new,
          })
          // Exact match: pre-fill with the resolved tag, zero friction. A
          // confident correction: pre-fill with the suggested tag as a
          // one-tap-to-accept default. Ambiguous/no-match: leave blank —
          // there's no safe default to guess at.
          setResolvedTag(needsResolution ? (preview.corrected ? (preview.candidates?.[0] ?? '') : '') : (preview.asset_tag ?? ''))
          setPhase('awaiting-accept')
          return
        }
        setResult({ kind: 'nothing' })
        setPhase('result')
        return
      }

      // Experiment: analyze the frame straight, then rotated, as a
      // cross-check against OCR misreads — a tag only one orientation's read
      // finds isn't silently trusted either way, it's flagged for the user
      // below instead. The rotation direction comes from the straight read
      // itself (Gemini judges which way makes the vertical asset tags most
      // upright), so this second call can't start until the first returns —
      // no parallelizing this one. Settings can turn this off (halving the
      // Gemini calls per scan); when disabled, `rotated` is just `straight`
      // again, which makes continueWithAssetTags's presence-agreement diff
      // a no-op and falls back to trusting a single read's tag resolutions.
      const straight = await api.reconcilePreview(blob)
      if (!straight.has_location_tag) {
        setResult({ kind: 'nothing' })
        setPhase('result')
        return
      }

      let rotated = straight
      if (dualReadEnabled) {
        const rotationDegrees = straight.suggested_rotation === 'counterclockwise' ? -90 : 90
        const rotatedBlob = await rotateSquareBlob(blob, rotationDegrees)
        rotated = await api.reconcilePreview(rotatedBlob)

        const tagsAgree = rotated.has_location_tag && straight.location_tag === rotated.location_tag
        if (!tagsAgree) {
          setResult({ kind: 'nothing' })
          setPhase('result')
          return
        }
      }

      // The location tag has to be resolved before the diff above (which was
      // computed against the raw, possibly-misread text) can be trusted —
      // see continueWithAssetTags's refetch-on-mismatch handling below.
      if (straight.location_tag_needs_resolution) {
        setLocationTagResolution({ straight, rotated })
        setResolvedLocationTag(straight.location_tag_corrected ? (straight.location_tag_candidates?.[0] ?? '') : '')
        setPhase('awaiting-location-tag')
        return
      }

      await continueWithAssetTags(straight.location_tag!, straight, rotated)
    } catch (err) {
      setResult({ kind: 'error', message: err instanceof ApiError ? err.message : 'Capture failed' })
      setPhase('result')
    }
  }

  async function onConfirmLocationTag() {
    if (!locationTagResolution || !LOCATION_TAG_PATTERN.test(resolvedLocationTag)) return
    const { straight, rotated } = locationTagResolution
    const locationTag = resolvedLocationTag
    setLocationTagResolution(null)
    setPhase('analyzing') // back to the spinner while the asset-tag step (and possibly a diff refetch) runs
    try {
      await continueWithAssetTags(locationTag, straight, rotated)
    } catch (err) {
      setResult({ kind: 'error', message: err instanceof ApiError ? err.message : 'Capture failed' })
      setPhase('result')
    }
  }

  function onCancelLocationTag() {
    setLocationTagResolution(null)
    resetToLive()
  }

  async function onConfirmTagReview(resolvedRowTags: string[]) {
    if (!tagReview) return
    setConfirmingTagReview(true)
    try {
      const assetTags = [...tagReview.agreedTags, ...resolvedRowTags]
      const diff = await api.reconcileDiff(tagReview.locationTag, assetTags)
      setTagReview(null)
      setConfirmingTagReview(false)
      if (diff.has_location_tag) {
        setPendingDiff(diff) // stays in 'analyzing' (spinner) until the modal is resolved
        return
      }
      setResult({ kind: 'nothing' })
      setPhase('result')
    } catch (err) {
      setResult({ kind: 'error', message: err instanceof ApiError ? err.message : 'Capture failed' })
      setTagReview(null)
      setConfirmingTagReview(false)
      setPhase('result')
    }
  }

  function onCancelTagReview() {
    setTagReview(null)
    resetToLive()
  }

  async function onAcceptCapture() {
    if (!pendingCapture || !capturedBlobRef.current || !ASSET_TAG_PATTERN.test(resolvedTag)) return
    setPhase('committing')
    try {
      await api.captureApply(capturedBlobRef.current, resolvedTag, acceptDescription ? pendingCapture.description : '')
      resetToLive() // saved successfully — clear everything and go straight back to a live, ready-to-shoot camera
    } catch (err) {
      setResult({ kind: 'error', message: err instanceof ApiError ? err.message : 'Save failed' })
      setPendingCapture(null)
      setPhase('result') // failed — keep the frozen frame + error up until the user acknowledges it
    }
  }

  async function onApproveReconcile(assetTags: string[]) {
    if (!pendingDiff?.location_tag) return
    setApplyingReconcile(true)
    try {
      await api.reconcileApply(pendingDiff.location_tag, assetTags)
      setPendingDiff(null)
      setApplyingReconcile(false)
      resetToLive() // applied successfully — clear everything and go straight back to a live, ready-to-shoot camera
    } catch (err) {
      setResult({ kind: 'error', message: err instanceof ApiError ? err.message : 'Reconciliation failed' })
      setPendingDiff(null)
      setApplyingReconcile(false)
      setPhase('result') // failed — keep the frozen frame + error up until the user acknowledges it
    }
  }

  function onCancelReconcile() {
    setPendingDiff(null)
    resetToLive()
  }

  const showingFrozenFrame = phase !== 'live'
  const busy = phase === 'analyzing' || phase === 'committing'

  return (
    <div class="capture-view">
      <Header active="capture" />

      <main class="capture-body">
        <div class="camera-square">
          {/* The video stays mounted at all times — its srcObject is only ever
              assigned once, on mount, so swapping it out for an <img> and back
              (as phase changes) would leave a freshly remounted <video> with
              no stream attached. The frozen-frame photo is instead layered on
              top of it and hidden/shown, never replacing it in the DOM. */}
          <video ref={videoRef} autoPlay playsInline muted class="camera-video" />
          {cameraError && <p class="camera-error">{cameraError}</p>}
          {!cameraError && showingFrozenFrame && frozenFrameUrl && (
            <img src={frozenFrameUrl} alt="" class="camera-frozen-frame" />
          )}
        </div>

        <div class="capture-toolbar">
          <div class="capture-mode-toggle" role="group" aria-label="Capture mode">
            <button
              type="button"
              class={'capture-mode-btn' + (mode === 'ingest' ? ' capture-mode-btn-active' : '')}
              onClick={() => setMode('ingest')}
              disabled={phase !== 'live'}
            >
              <Icon icon={faCamera} /> Ingest item
            </button>
            <button
              type="button"
              class={'capture-mode-btn' + (mode === 'locate' ? ' capture-mode-btn-active' : '')}
              onClick={() => setMode('locate')}
              disabled={phase !== 'live'}
            >
              <Icon icon={faMap} /> Locate items
            </button>
          </div>

          {mode === 'ingest' && phase === 'awaiting-accept' && pendingCapture && (
            <label class="capture-accept-description">
              <input
                type="checkbox"
                checked={acceptDescription}
                onChange={(e) => setAcceptDescription((e.target as HTMLInputElement).checked)}
              />
              Desc
            </label>
          )}
        </div>

        <div class="capture-results">
          {phase === 'analyzing' && !pendingDiff && !tagReview && <p class="capture-feedback">Analyzing photo…</p>}
          {phase === 'committing' && <p class="capture-feedback">Saving…</p>}

          {phase === 'awaiting-accept' && pendingCapture && (
            <div class="capture-result-card">
              {pendingCapture.needsResolution ? (
                <div class="capture-tag-resolver">
                  <p class="capture-tag-resolver-hint">
                    OCR read <strong>{pendingCapture.rawAssetTag}</strong>
                    {pendingCapture.corrected ? ' — pick the correct tag:' : ' — no confident match, pick or type the correct tag:'}
                  </p>
                  <div class="capture-tag-choices">
                    {[...new Set([pendingCapture.rawAssetTag, ...pendingCapture.candidates])].map((choice) => (
                      <button
                        type="button"
                        class={'capture-tag-choice' + (resolvedTag === choice ? ' capture-tag-choice-active' : '')}
                        onClick={() => setResolvedTag(choice)}
                        key={choice}
                      >
                        {choice}
                      </button>
                    ))}
                  </div>
                  <label class="capture-tag-manual">
                    Or type the correct tag
                    <input
                      type="text"
                      maxLength={4}
                      value={resolvedTag}
                      onInput={(e) => setResolvedTag((e.target as HTMLInputElement).value.toUpperCase())}
                    />
                  </label>
                </div>
              ) : (
                <div class="capture-result-header">
                  <span class="capture-result-tag">{pendingCapture.assetTag}</span>
                  <span class="capture-result-action">
                    {pendingCapture.itemWillBeNew ? 'Will add new item' : 'Will add new photo'}
                  </span>
                </div>
              )}
              {pendingCapture.guess && <p class="capture-result-guess">{pendingCapture.guess}</p>}
              <p class={'capture-result-description' + (acceptDescription ? '' : ' capture-result-description-rejected')}>
                {pendingCapture.description || <em>No notes read from this photo.</em>}
              </p>
              <p class="capture-result-hint">Accept to save, or Cancel to discard this photo.</p>
            </div>
          )}

          {phase === 'awaiting-location-tag' && locationTagResolution && (
            <div class="capture-result-card">
              <div class="capture-tag-resolver">
                <p class="capture-tag-resolver-hint">
                  OCR read <strong>{locationTagResolution.straight.raw_location_tag}</strong>
                  {locationTagResolution.straight.location_tag_corrected
                    ? ' — pick the correct location tag:'
                    : ' — no confident match, pick or type the correct location tag:'}
                </p>
                <div class="capture-tag-choices">
                  {[
                    ...new Set([
                      locationTagResolution.straight.raw_location_tag ?? '',
                      ...(locationTagResolution.straight.location_tag_candidates ?? []),
                    ]),
                  ].map((choice) => (
                    <button
                      type="button"
                      class={'capture-tag-choice' + (resolvedLocationTag === choice ? ' capture-tag-choice-active' : '')}
                      onClick={() => setResolvedLocationTag(choice)}
                      key={choice}
                    >
                      {choice}
                    </button>
                  ))}
                </div>
                <label class="capture-tag-manual">
                  Or type the correct location tag
                  <input
                    type="text"
                    maxLength={4}
                    value={resolvedLocationTag}
                    onInput={(e) => setResolvedLocationTag((e.target as HTMLInputElement).value.toUpperCase())}
                  />
                </label>
              </div>
              <p class="capture-result-hint">Confirm the location tag to see the reconciliation diff.</p>
            </div>
          )}

          {result?.kind === 'nothing' && (
            <p class="capture-feedback capture-feedback-warning">
              {mode === 'ingest'
                ? 'No asset tag found — retake with the label clearly visible.'
                : 'No location tag found — retake with the label clearly visible.'}
            </p>
          )}
          {result?.kind === 'error' && <p class="capture-feedback capture-feedback-error">{result.message}</p>}
        </div>
      </main>

      <div class="capture-controls">
        {phase === 'awaiting-accept' || phase === 'awaiting-location-tag' ? (
          <>
            <button
              type="button"
              class="capture-button capture-button-cancel"
              onClick={phase === 'awaiting-location-tag' ? onCancelLocationTag : resetToLive}
              aria-label="Cancel — discard this photo"
            >
              <Icon icon={faXmark} />
            </button>
            <button
              type="button"
              class="capture-button capture-button-accept"
              onClick={phase === 'awaiting-location-tag' ? onConfirmLocationTag : onAcceptCapture}
              disabled={phase === 'awaiting-location-tag' ? !LOCATION_TAG_PATTERN.test(resolvedLocationTag) : !ASSET_TAG_PATTERN.test(resolvedTag)}
              aria-label={phase === 'awaiting-location-tag' ? 'Confirm location tag' : 'Accept — save this item'}
            >
              <Icon icon={faCheck} />
            </button>
          </>
        ) : (
          <button
            type="button"
            class="capture-button"
            onClick={phase === 'result' ? resetToLive : onCapture}
            disabled={busy || (phase === 'live' && !!cameraError)}
            aria-label={phase === 'result' ? 'Clear and capture another photo' : 'Capture photo'}
          >
            {busy && <span class="capture-spinner" />}
            {phase === 'result' && <Icon icon={faXmark} />}
          </button>
        )}
      </div>

      {pendingDiff && (
        <ReconcileDiff
          diff={pendingDiff}
          applying={applyingReconcile}
          onApprove={onApproveReconcile}
          onCancel={onCancelReconcile}
          correctedTags={correctedTags}
        />
      )}

      {tagReview && (
        <TagAgreementReview
          locationTag={tagReview.locationTag}
          agreedTags={tagReview.agreedTags}
          rows={tagReview.rows}
          confirming={confirmingTagReview}
          onConfirm={onConfirmTagReview}
          onCancel={onCancelTagReview}
        />
      )}

      <Footer />
    </div>
  )
}
