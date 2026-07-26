import { useEffect, useRef, useState } from 'preact/hooks'
import { api, ApiError, type CaptureResponse, type ReconcileDiffResponse } from '../api/client'
import { captureSquareFrame } from '../lib/camera'
import { currentUser, logout } from '../state/auth'
import { ReconcileDiff } from '../components/ReconcileDiff'

interface RouteProps {
  path?: string
  default?: boolean
}

type Feedback =
  | { kind: 'none' }
  | { kind: 'success'; response: CaptureResponse }
  | { kind: 'reconciled'; diff: ReconcileDiffResponse }
  | { kind: 'nothing-found' }
  | { kind: 'error'; message: string }

export function Capture(_props: RouteProps) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const [cameraError, setCameraError] = useState<string | null>(null)
  const [capturing, setCapturing] = useState(false)
  const [feedback, setFeedback] = useState<Feedback>({ kind: 'none' })
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

  async function onCapture() {
    if (!videoRef.current || capturing) return
    setCapturing(true)
    setFeedback({ kind: 'none' })
    try {
      const blob = await captureSquareFrame(videoRef.current)

      // The app doesn't ask which kind of label is in frame — it tries the
      // asset-tag flow first (the common case), and only falls back to the
      // location-reconciliation flow if no asset tag was found.
      const captureResult = await api.capture(blob)
      if (captureResult.has_asset_tag) {
        setFeedback({ kind: 'success', response: captureResult })
        return
      }

      const diff = await api.reconcilePreview(blob)
      if (diff.has_location_code) {
        setPendingDiff(diff)
        return
      }

      setFeedback({ kind: 'nothing-found' })
    } catch (err) {
      setFeedback({ kind: 'error', message: err instanceof ApiError ? err.message : 'Capture failed' })
    } finally {
      setCapturing(false)
    }
  }

  async function onApproveReconcile() {
    if (!pendingDiff?.location_code) return
    setApplying(true)
    try {
      const applied = await api.reconcileApply(pendingDiff.location_code, pendingDiff.asset_tags)
      setFeedback({ kind: 'reconciled', diff: applied })
      setPendingDiff(null)
    } catch (err) {
      setFeedback({ kind: 'error', message: err instanceof ApiError ? err.message : 'Reconciliation failed' })
      setPendingDiff(null)
    } finally {
      setApplying(false)
    }
  }

  return (
    <div class="capture-view">
      <header class="app-header">
        <span class="app-title">aiinventory</span>
        <span class="app-header-user">
          {currentUser.value?.username}
          <a href="/search">Search</a>
          <a href="/settings">Settings</a>
          <button type="button" class="link-button" onClick={() => logout()}>
            Sign out
          </button>
        </span>
      </header>

      <main class="capture-body">
        <div class="camera-square">
          {cameraError ? (
            <p class="camera-error">{cameraError}</p>
          ) : (
            <video ref={videoRef} autoPlay playsInline muted class="camera-video" />
          )}

          <button
            type="button"
            class="capture-button"
            onClick={onCapture}
            disabled={capturing || !!cameraError}
            aria-label="Capture photo"
          />
        </div>

        {feedback.kind === 'success' && (
          <p class="capture-feedback capture-feedback-success">
            {feedback.response.item_was_new ? 'Created new item ' : 'Added photo to '}
            <strong>{feedback.response.asset_tag}</strong>
            {feedback.response.item_guess ? ` — ${feedback.response.item_guess}` : ''}
          </p>
        )}
        {feedback.kind === 'reconciled' && (
          <p class="capture-feedback capture-feedback-success">
            Reconciled <strong>{feedback.diff.location_code}</strong>: +{feedback.diff.added.length} ~
            {feedback.diff.moved.length} -{feedback.diff.removed.length}
          </p>
        )}
        {feedback.kind === 'nothing-found' && (
          <p class="capture-feedback capture-feedback-warning">
            No asset tag or location code found — retake with the label clearly visible.
          </p>
        )}
        {feedback.kind === 'error' && <p class="capture-feedback capture-feedback-error">{feedback.message}</p>}
      </main>

      {pendingDiff && (
        <ReconcileDiff
          diff={pendingDiff}
          applying={applying}
          onApprove={onApproveReconcile}
          onCancel={() => setPendingDiff(null)}
        />
      )}
    </div>
  )
}
