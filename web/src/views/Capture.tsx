import { useEffect, useRef, useState } from 'preact/hooks'
import { api, ApiError, type CaptureResponse } from '../api/client'
import { captureSquareFrame } from '../lib/camera'
import { currentUser, logout } from '../state/auth'

interface RouteProps {
  path?: string
  default?: boolean
}

type Feedback =
  | { kind: 'none' }
  | { kind: 'success'; response: CaptureResponse }
  | { kind: 'no-tag' }
  | { kind: 'error'; message: string }

export function Capture(_props: RouteProps) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const [cameraError, setCameraError] = useState<string | null>(null)
  const [capturing, setCapturing] = useState(false)
  const [feedback, setFeedback] = useState<Feedback>({ kind: 'none' })

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
      const response = await api.capture(blob)
      setFeedback(response.has_asset_tag ? { kind: 'success', response } : { kind: 'no-tag' })
    } catch (err) {
      setFeedback({ kind: 'error', message: err instanceof ApiError ? err.message : 'Capture failed' })
    } finally {
      setCapturing(false)
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
        {feedback.kind === 'no-tag' && (
          <p class="capture-feedback capture-feedback-warning">
            No asset tag found — retake with the tag clearly visible.
          </p>
        )}
        {feedback.kind === 'error' && <p class="capture-feedback capture-feedback-error">{feedback.message}</p>}
      </main>
    </div>
  )
}
