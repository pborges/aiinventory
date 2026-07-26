import type { ReconcileDiffResponse } from '../api/client'

interface Props {
  diff: ReconcileDiffResponse
  onApprove: () => void
  onCancel: () => void
  applying: boolean
}

/** The git-diff-style approval overlay for README flow #2 — nothing is
 * written until the user explicitly approves. */
export function ReconcileDiff({ diff, onApprove, onCancel, applying }: Props) {
  const noChanges = diff.added.length === 0 && diff.moved.length === 0 && diff.removed.length === 0

  return (
    <div class="modal-overlay">
      <div class="modal-panel">
        <h2>Reconciling {diff.location_code}</h2>

        {noChanges ? (
          <p>No changes — this location already matches the frame.</p>
        ) : (
          <ul class="reconcile-diff-list">
            {diff.added.map((tag) => (
              <li class="diff-added" key={`a-${tag}`}>
                + {tag} added
              </li>
            ))}
            {diff.moved.map((m) => (
              <li class="diff-moved" key={`m-${m.asset_tag}`}>
                ~ {m.asset_tag} moved{m.from_location ? ` (was ${m.from_location})` : ''}
              </li>
            ))}
            {diff.removed.map((tag) => (
              <li class="diff-removed" key={`r-${tag}`}>
                - {tag} removed
              </li>
            ))}
          </ul>
        )}

        <div class="modal-actions">
          <button type="button" onClick={onCancel} disabled={applying}>
            Cancel
          </button>
          <button type="button" onClick={onApprove} disabled={applying || noChanges}>
            {applying ? 'Applying…' : 'Approve'}
          </button>
        </div>
      </div>
    </div>
  )
}
