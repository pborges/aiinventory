import { useState } from 'preact/hooks'

interface Props {
  locationCode: string
  agreedTags: string[]
  diffTags: string[]
  onConfirm: (selectedDiffTags: string[]) => void
  onCancel: () => void
  confirming: boolean
}

/** Shown for the locate flow's straight-vs-rotated dual-read experiment when
 * the two analyses of the same frame don't fully agree on which asset tags
 * are visible. Tags both reads agree on are taken as given; tags only one
 * read found are listed here for the user to individually confirm before a
 * diff is computed. */
export function TagAgreementReview({ locationCode, agreedTags, diffTags, onConfirm, onCancel, confirming }: Props) {
  const [selected, setSelected] = useState<Set<string>>(new Set())

  function toggle(tag: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(tag)) next.delete(tag)
      else next.add(tag)
      return next
    })
  }

  return (
    <div class="modal-overlay">
      <div class="modal-panel">
        <h2>Confirm tags at {locationCode}</h2>
        <p class="tag-review-hint">
          The straight and rotated reads of this photo didn't fully agree. Select the tags below that you can
          actually see.
        </p>

        {agreedTags.length > 0 && (
          <ul class="tag-review-agreed">
            {agreedTags.map((tag) => (
              <li key={tag}>{tag}</li>
            ))}
          </ul>
        )}

        <ul class="tag-review-list">
          {diffTags.map((tag) => (
            <li class="tag-review-card" key={tag}>
              <label>
                <input
                  type="checkbox"
                  checked={selected.has(tag)}
                  onChange={() => toggle(tag)}
                  disabled={confirming}
                />
                <span class="tag-review-tag">{tag}</span>
              </label>
            </li>
          ))}
        </ul>

        <div class="modal-actions">
          <button type="button" onClick={onCancel} disabled={confirming}>
            Cancel
          </button>
          <button
            type="button"
            class="btn-primary"
            onClick={() => onConfirm([...selected])}
            disabled={confirming}
          >
            {confirming ? 'Checking…' : 'Continue'}
          </button>
        </div>
      </div>
    </div>
  )
}
