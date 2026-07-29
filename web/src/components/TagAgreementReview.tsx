import { useState } from 'preact/hooks'

export interface TagReviewRow {
  raw: string
  /** Registry-suggested candidates for raw, closest first — empty when the
   * row exists purely because of a straight/rotated presence disagreement
   * with no spelling ambiguity. */
  candidates: string[]
}

interface Props {
  locationTag: string
  agreedTags: string[]
  rows: TagReviewRow[]
  onConfirm: (resolvedTags: string[]) => void
  onCancel: () => void
  confirming: boolean
}

const ASSET_TAG_PATTERN = /^[A-Z]{4}$/

/** Shown whenever a tag can't be trusted outright: either the locate flow's
 * straight-vs-rotated dual-read experiment disagreed on presence, or a read
 * (agreed-on or not) doesn't exactly match anything in the tag registry.
 * Each row is a single choice — exclude, one of the raw/suggested values, or
 * a manually typed tag — never a silent auto-pick, since neither the dual
 * read nor a confident registry correction can tell "OCR misread of an
 * existing tag" apart from "a genuinely different, coincidentally similar
 * new tag." Tags both reads agreed on AND matched the registry exactly skip
 * this review entirely. */
export function TagAgreementReview({ locationTag, agreedTags, rows, onConfirm, onCancel, confirming }: Props) {
  // raw -> chosen final tag value ('' means excluded/unset, the default)
  const [selected, setSelected] = useState<Record<string, string>>({})
  const [manual, setManual] = useState<Record<string, string>>({})

  function choose(raw: string, value: string) {
    setSelected((prev) => ({ ...prev, [raw]: value }))
  }

  function setManualValue(raw: string, value: string) {
    const upper = value.toUpperCase()
    setManual((prev) => ({ ...prev, [raw]: upper }))
    setSelected((prev) => ({ ...prev, [raw]: upper }))
  }

  const resolvedValues = rows.map((row) => selected[row.raw] ?? '').filter((v) => v !== '')
  const allValid = resolvedValues.every((v) => ASSET_TAG_PATTERN.test(v))

  return (
    <div class="modal-overlay">
      <div class="modal-panel">
        <h2>Confirm tags at {locationTag}</h2>
        <p class="tag-review-hint">
          Some tags need a closer look — either the straight and rotated reads didn't fully agree, or a read
          doesn't exactly match a registered tag. Pick the correct tag for each, or exclude it.
        </p>

        {agreedTags.length > 0 && (
          <ul class="tag-review-agreed">
            {agreedTags.map((tag) => (
              <li key={tag}>{tag}</li>
            ))}
          </ul>
        )}

        <ul class="tag-review-list">
          {rows.map((row) => {
            const choices = [...new Set([row.raw, ...row.candidates])]
            const current = selected[row.raw] ?? ''
            return (
              <li class="tag-review-card" key={row.raw}>
                <div class="tag-review-card-header">
                  Read as <span class="tag-review-tag">{row.raw}</span>
                </div>
                <div class="tag-review-choices">
                  <button
                    type="button"
                    class={'tag-review-choice tag-review-choice-exclude' + (current === '' ? ' tag-review-choice-active' : '')}
                    onClick={() => choose(row.raw, '')}
                    disabled={confirming}
                  >
                    Exclude
                  </button>
                  {choices.map((choice) => (
                    <button
                      type="button"
                      class={'tag-review-choice' + (current === choice ? ' tag-review-choice-active' : '')}
                      onClick={() => choose(row.raw, choice)}
                      disabled={confirming}
                      key={choice}
                    >
                      {choice}
                    </button>
                  ))}
                </div>
                <label class="tag-review-manual">
                  Or type the correct tag
                  <input
                    type="text"
                    maxLength={4}
                    value={manual[row.raw] ?? ''}
                    onInput={(e) => setManualValue(row.raw, (e.target as HTMLInputElement).value)}
                    disabled={confirming}
                  />
                </label>
              </li>
            )
          })}
        </ul>

        <div class="modal-actions">
          <button type="button" onClick={onCancel} disabled={confirming}>
            Cancel
          </button>
          <button
            type="button"
            class="btn-primary"
            onClick={() => onConfirm(resolvedValues)}
            disabled={confirming || !allValid}
          >
            {confirming ? 'Checking…' : 'Continue'}
          </button>
        </div>
      </div>
    </div>
  )
}
