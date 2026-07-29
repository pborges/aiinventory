export const CAPTURE_MAX_DIMENSION = 1600
export const CAPTURE_JPEG_QUALITY = 0.85

/**
 * Center-crops the current video frame to a square and downsizes it to a
 * bounded max dimension before it goes anywhere — the same optimized bytes
 * are what get uploaded, sent to Gemini, and stored (see README's Tech stack).
 */
export function captureSquareFrame(video: HTMLVideoElement): Promise<Blob> {
  const srcSize = Math.min(video.videoWidth, video.videoHeight)
  const sx = (video.videoWidth - srcSize) / 2
  const sy = (video.videoHeight - srcSize) / 2

  const outSize = Math.min(srcSize, CAPTURE_MAX_DIMENSION)
  const canvas = document.createElement('canvas')
  canvas.width = outSize
  canvas.height = outSize
  const ctx = canvas.getContext('2d')
  if (!ctx) {
    return Promise.reject(new Error('canvas 2d context unavailable'))
  }
  ctx.drawImage(video, sx, sy, srcSize, srcSize, 0, 0, outSize, outSize)

  return new Promise((resolve, reject) => {
    canvas.toBlob(
      (blob) => (blob ? resolve(blob) : reject(new Error('image encoding failed'))),
      'image/jpeg',
      CAPTURE_JPEG_QUALITY,
    )
  })
}

/**
 * Rotates a square JPEG blob by the given number of degrees (dimensions are
 * unchanged since the source is square). Used for the locate-flow dual-read
 * experiment: analyzing the same frame straight and rotated 90° is a second,
 * independent OCR pass that can catch a misread the other orientation
 * happened to get right.
 */
export function rotateSquareBlob(blob: Blob, degrees: number): Promise<Blob> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(blob)
    const img = new Image()
    img.onload = () => {
      URL.revokeObjectURL(url)
      const canvas = document.createElement('canvas')
      canvas.width = img.width
      canvas.height = img.height
      const ctx = canvas.getContext('2d')
      if (!ctx) {
        reject(new Error('canvas 2d context unavailable'))
        return
      }
      ctx.translate(canvas.width / 2, canvas.height / 2)
      ctx.rotate((degrees * Math.PI) / 180)
      ctx.drawImage(img, -img.width / 2, -img.height / 2)
      canvas.toBlob(
        (out) => (out ? resolve(out) : reject(new Error('image encoding failed'))),
        'image/jpeg',
        CAPTURE_JPEG_QUALITY,
      )
    }
    img.onerror = () => {
      URL.revokeObjectURL(url)
      reject(new Error('image decoding failed'))
    }
    img.src = url
  })
}
