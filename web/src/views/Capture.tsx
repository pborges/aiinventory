import { useEffect, useRef, useState } from 'preact/hooks'
import { api, ApiError, type ReconcileDiffResponse } from '../api/client'
import { captureSquareFrame } from '../lib/camera'
import { ReconcileDiff } from '../components/ReconcileDiff'
import { Header } from '../components/Header'

interface RouteProps {
  path?: string
  default?: boolean
}

type Phase = 'live' | 'processing' | 'result'

type ResultData =
  | { kind: 'item'; assetTag: string; itemId: number; itemWasNew: boolean; guess: string; description: string }
  | { kind: 'reconciled'; diff: ReconcileDiffResponse }
  | { kind: 'reconcile-cancelled' }
  | { kind: 'nothing' }
  | { kind: 'error'; message: string }

export function Capture(_props: RouteProps) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const [cameraError, setCameraError] = useState<string | null>(null)
  const [phase, setPhase] = useState<Phase>('live')
  const [frozenFrameUrl, setFrozenFrameUrl] = useState<string | null>(null)
  const [result, setResult] = useState<ResultData | null>(null)
  const [pendingDiff, setPendingDiff] = useState<ReconcileDiffResponse | null>(null)
  const [applying, setApplying] = useState(false)

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

  async function onCapture() {
    if (!videoRef.current || phase !== 'live') return

    const blob = await captureSquareFrame(videoRef.current)
    setFrozenFrameUrl(URL.createObjectURL(blob))
    setPhase('processing')

    try {
      // The app doesn't ask which kind of label is in frame — it tries the
      // asset-tag flow first (the common case), and only falls back to the
      // location-reconciliation flow if no asset tag was found.
      const captureResult = await api.capture(blob)
      if (captureResult.has_asset_tag) {
        setResult({
          kind: 'item',
          assetTag: captureResult.asset_tag ?? '',
          itemId: captureResult.item_id ?? 0,
          itemWasNew: !!captureResult.item_was_new,
          guess: captureResult.item_guess ?? '',
          description: captureResult.image_description ?? '',
        })
        setPhase('result')
        return
      }

      const diff = await api.reconcilePreview(blob)
      if (diff.has_location_code) {
        setPendingDiff(diff) // stays in 'processing' (spinner) until the modal is resolved
        return
      }

      setResult({ kind: 'nothing' })
      setPhase('result')
    } catch (err) {
      setResult({ kind: 'error', message: err instanceof ApiError ? err.message : 'Capture failed' })
      setPhase('result')
    }
  }

  async function onApproveReconcile() {
    if (!pendingDiff?.location_code) return
    setApplying(true)
    try {
      const applied = await api.reconcileApply(pendingDiff.location_code, pendingDiff.asset_tags)
      setResult({ kind: 'reconciled', diff: applied })
    } catch (err) {
      setResult({ kind: 'error', message: err instanceof ApiError ? err.message : 'Reconciliation failed' })
    } finally {
      setPendingDiff(null)
      setApplying(false)
      setPhase('result')
    }
  }

  function onCancelReconcile() {
    setPendingDiff(null)
    setResult({ kind: 'reconcile-cancelled' })
    setPhase('result')
  }

  function onClear() {
    setFrozenFrameUrl(null)
    setResult(null)
    setPhase('live')
  }

  const showingFrozenFrame = phase === 'processing' || phase === 'result'

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

        <div class="capture-results">
          {phase === 'processing' && !pendingDiff && <p class="capture-feedback">Analyzing photo…</p>}

          {result?.kind === 'item' && (
            <div class="capture-result-card">
              <div class="capture-result-header">
                <a href={`/items/${result.itemId}`} class="capture-result-tag">
                  {result.assetTag}
                </a>
                <span class="capture-result-action">{result.itemWasNew ? 'Added new item' : 'Added new photo'}</span>
              </div>
              {result.guess && <p class="capture-result-guess">{result.guess}</p>}
              <p class="capture-result-description">{result.description || <em>No notes read from this photo.</em>}</p>
              <p class="capture-result-hint">Tap the button below to capture another photo.</p>
            </div>
          )}

          {result?.kind === 'reconciled' && (
            <div class="capture-result-card">
              <div class="capture-result-header">
                <span class="capture-result-tag">{result.diff.location_code}</span>
                <span class="capture-result-action">Reconciled location</span>
              </div>
              <ul class="capture-result-diff-list">
                {result.diff.added.map((tag) => (
                  <li class="diff-added" key={`a-${tag}`}>
                    + {tag} added
                  </li>
                ))}
                {result.diff.moved.map((m) => (
                  <li class="diff-moved" key={`m-${m.asset_tag}`}>
                    ~ {m.asset_tag} moved{m.from_location ? ` (was ${m.from_location})` : ''}
                  </li>
                ))}
                {result.diff.removed.map((tag) => (
                  <li class="diff-removed" key={`r-${tag}`}>
                    - {tag} removed
                  </li>
                ))}
              </ul>
              <p class="capture-result-hint">Tap the button below to capture another photo.</p>
            </div>
          )}

          {result?.kind === 'reconcile-cancelled' && (
            <p class="capture-feedback capture-feedback-warning">Reconciliation cancelled.</p>
          )}
          {result?.kind === 'nothing' && (
            <p class="capture-feedback capture-feedback-warning">
              No asset tag or location code found — retake with the label clearly visible.
            </p>
          )}
          {result?.kind === 'error' && <p class="capture-feedback capture-feedback-error">{result.message}</p>}
        </div>
      </main>

      <button
        type="button"
        class="capture-button"
        onClick={phase === 'result' ? onClear : onCapture}
        disabled={phase === 'processing' || (phase === 'live' && !!cameraError)}
        aria-label={phase === 'result' ? 'Clear and capture another photo' : 'Capture photo'}
      >
        {phase === 'processing' && <span class="capture-spinner" />}
        {phase === 'result' && <span aria-hidden="true">✕</span>}
      </button>

      {pendingDiff && (
        <ReconcileDiff
          diff={pendingDiff}
          applying={applying}
          onApprove={onApproveReconcile}
          onCancel={onCancelReconcile}
        />
      )}
    </div>
  )
}
