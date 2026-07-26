import { useEffect, useRef, useState } from 'preact/hooks'
import { faCamera, faMap, faXmark, faCheck } from '@fortawesome/free-solid-svg-icons'
import { api, ApiError, type ReconcileDiffResponse } from '../api/client'
import { captureSquareFrame } from '../lib/camera'
import { ReconcileDiff } from '../components/ReconcileDiff'
import { Header } from '../components/Header'
import { Footer } from '../components/Footer'
import { Icon } from '../components/Icon'

interface RouteProps {
  path?: string
  default?: boolean
}

type Phase = 'live' | 'analyzing' | 'awaiting-accept' | 'committing' | 'result'
type CaptureMode = 'ingest' | 'locate'

interface PendingCapture {
  assetTag: string
  guess: string
  description: string
  itemWillBeNew: boolean
}

type ResultData = { kind: 'nothing' } | { kind: 'error'; message: string }

export function Capture(_props: RouteProps) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const capturedBlobRef = useRef<Blob | null>(null)
  const [cameraError, setCameraError] = useState<string | null>(null)
  const [phase, setPhase] = useState<Phase>('live')
  const [mode, setMode] = useState<CaptureMode>('ingest')
  const [frozenFrameUrl, setFrozenFrameUrl] = useState<string | null>(null)
  const [pendingCapture, setPendingCapture] = useState<PendingCapture | null>(null)
  const [result, setResult] = useState<ResultData | null>(null)
  const [pendingDiff, setPendingDiff] = useState<ReconcileDiffResponse | null>(null)
  const [applyingReconcile, setApplyingReconcile] = useState(false)

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
    setResult(null)
    capturedBlobRef.current = null
    setPhase('live')
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
      // the viewfinder), not auto-detected — asset tags and location codes
      // can both be a handful of uppercase letters on a white sticker, and
      // guessing which one Gemini should look for was unreliable in
      // practice. Analyzing a tag never writes anything by itself; the
      // user must Accept.
      if (mode === 'ingest') {
        const preview = await api.capturePreview(blob)
        if (preview.has_asset_tag) {
          setPendingCapture({
            assetTag: preview.asset_tag ?? '',
            guess: preview.item_guess ?? '',
            description: preview.image_description ?? '',
            itemWillBeNew: !!preview.item_will_be_new,
          })
          setPhase('awaiting-accept')
          return
        }
        setResult({ kind: 'nothing' })
        setPhase('result')
        return
      }

      const diff = await api.reconcilePreview(blob)
      if (diff.has_location_code) {
        setPendingDiff(diff) // stays in 'analyzing' (spinner) until the modal is resolved
        return
      }

      setResult({ kind: 'nothing' })
      setPhase('result')
    } catch (err) {
      setResult({ kind: 'error', message: err instanceof ApiError ? err.message : 'Capture failed' })
      setPhase('result')
    }
  }

  async function onAcceptCapture() {
    if (!pendingCapture || !capturedBlobRef.current) return
    setPhase('committing')
    try {
      await api.captureApply(capturedBlobRef.current, pendingCapture.assetTag, pendingCapture.description)
      resetToLive() // saved successfully — clear everything and go straight back to a live, ready-to-shoot camera
    } catch (err) {
      setResult({ kind: 'error', message: err instanceof ApiError ? err.message : 'Save failed' })
      setPendingCapture(null)
      setPhase('result') // failed — keep the frozen frame + error up until the user acknowledges it
    }
  }

  async function onApproveReconcile(assetTags: string[]) {
    if (!pendingDiff?.location_code) return
    setApplyingReconcile(true)
    try {
      await api.reconcileApply(pendingDiff.location_code, assetTags)
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

        <div class="capture-results">
          {phase === 'analyzing' && !pendingDiff && <p class="capture-feedback">Analyzing photo…</p>}
          {phase === 'committing' && <p class="capture-feedback">Saving…</p>}

          {phase === 'awaiting-accept' && pendingCapture && (
            <div class="capture-result-card">
              <div class="capture-result-header">
                <span class="capture-result-tag">{pendingCapture.assetTag}</span>
                <span class="capture-result-action">
                  {pendingCapture.itemWillBeNew ? 'Will add new item' : 'Will add new photo'}
                </span>
              </div>
              {pendingCapture.guess && <p class="capture-result-guess">{pendingCapture.guess}</p>}
              <p class="capture-result-description">
                {pendingCapture.description || <em>No notes read from this photo.</em>}
              </p>
              <p class="capture-result-hint">Accept to save, or Cancel to discard this photo.</p>
            </div>
          )}

          {result?.kind === 'nothing' && (
            <p class="capture-feedback capture-feedback-warning">
              {mode === 'ingest'
                ? 'No asset tag found — retake with the label clearly visible.'
                : 'No location code found — retake with the label clearly visible.'}
            </p>
          )}
          {result?.kind === 'error' && <p class="capture-feedback capture-feedback-error">{result.message}</p>}
        </div>
      </main>

      <div class="capture-controls">
        {phase === 'awaiting-accept' ? (
          <>
            <button
              type="button"
              class="capture-button capture-button-cancel"
              onClick={resetToLive}
              aria-label="Cancel — discard this photo"
            >
              <Icon icon={faXmark} />
            </button>
            <button
              type="button"
              class="capture-button capture-button-accept"
              onClick={onAcceptCapture}
              aria-label="Accept — save this item"
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
        />
      )}

      <Footer />
    </div>
  )
}
