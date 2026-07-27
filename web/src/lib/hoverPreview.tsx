import { useState } from 'preact/hooks'

// Follows the cursor rather than growing the thumbnail in place — an
// in-place hover-zoom (tried first) covers the row's own tag/description
// text, since the thumb sits directly beside them in the same flex row.
const HOVER_PREVIEW_SIZE = 640
const HOVER_PREVIEW_MARGIN = 16

interface HoverPreviewState {
  src: string
  x: number
  y: number
}

export function useHoverPreview() {
  const [preview, setPreview] = useState<HoverPreviewState | null>(null)

  function showHoverPreview(src: string, e: { clientX: number; clientY: number }) {
    let x = e.clientX + HOVER_PREVIEW_MARGIN
    let y = e.clientY + HOVER_PREVIEW_MARGIN
    if (x + HOVER_PREVIEW_SIZE > window.innerWidth) x = e.clientX - HOVER_PREVIEW_SIZE - HOVER_PREVIEW_MARGIN
    if (y + HOVER_PREVIEW_SIZE > window.innerHeight) y = window.innerHeight - HOVER_PREVIEW_SIZE - HOVER_PREVIEW_MARGIN
    if (y < HOVER_PREVIEW_MARGIN) y = HOVER_PREVIEW_MARGIN
    setPreview({ src, x, y })
  }

  function hideHoverPreview() {
    setPreview(null)
  }

  return { preview, showHoverPreview, hideHoverPreview }
}

export function HoverPreview({ preview }: { preview: HoverPreviewState | null }) {
  if (!preview) return null
  return <img src={preview.src} alt="" class="hover-preview" style={{ left: `${preview.x}px`, top: `${preview.y}px` }} />
}
