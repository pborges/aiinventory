import { useState } from 'preact/hooks'
import type { ReconcileDiffResponse } from '../api/client'

interface Props {
  diff: ReconcileDiffResponse
  onApprove: (assetTags: string[]) => void
  onCancel: () => void
  applying: boolean
}

type DiffKind = 'new' | 'added' | 'moved' | 'removed'

interface DiffCard {
  kind: DiffKind
  assetTag: string
  fromLocation?: string
}

const KIND_LABEL: Record<DiffKind, string> = {
  new: 'New item',
  added: 'Added',
  moved: 'Moved',
  removed: 'Removed',
}

/** The git-diff-style approval overlay for README flow #2 — nothing is
 * written until the user explicitly approves. Each proposed change is its
 * own card with a trash icon to drop just that one change before approving;
 * dropping a new/added/moved card excludes that tag from what gets sent on
 * apply, dropping a removed card keeps that tag linked here instead. */
export function ReconcileDiff({ diff, onApprove, onCancel, applying }: Props) {
  const [dismissed, setDismissed] = useState<Set<string>>(new Set())

  const cards: DiffCard[] = [
    ...diff.new.map((tag) => ({ kind: 'new' as const, assetTag: tag })),
    ...diff.added.map((tag) => ({ kind: 'added' as const, assetTag: tag })),
    ...diff.moved.map((m) => ({ kind: 'moved' as const, assetTag: m.asset_tag, fromLocation: m.from_location })),
    ...diff.removed.map((tag) => ({ kind: 'removed' as const, assetTag: tag })),
  ].filter((card) => !dismissed.has(card.assetTag))

  const noChanges = cards.length === 0

  function onDismiss(assetTag: string) {
    setDismissed((prev) => new Set(prev).add(assetTag))
  }

  function onApproveClick() {
    // asset_tags is the full frame tag list the backend re-diffs against on
    // apply (see internal/inventory.ComputeReconciliation) — dropping a
    // new/added/moved card means excluding its tag from that list, and
    // dropping a removed card means the opposite: adding its tag back in so
    // it reads as "still here, no change" instead of "gone".
    const assetTags = [
      ...diff.asset_tags.filter((tag) => !dismissed.has(tag)),
      ...diff.removed.filter((tag) => dismissed.has(tag)),
    ]
    onApprove(assetTags)
  }

  return (
    <div class="modal-overlay">
      <div class="modal-panel">
        <h2>Reconciling {diff.location_code}</h2>

        {noChanges ? (
          <p>No changes — this location already matches the frame.</p>
        ) : (
          <ul class="reconcile-diff-list">
            {cards.map((card) => (
              <li class={`reconcile-diff-card diff-${card.kind}`} key={`${card.kind}-${card.assetTag}`}>
                <span class="reconcile-diff-kind">{KIND_LABEL[card.kind]}</span>
                <span class="reconcile-diff-tag">{card.assetTag}</span>
                {card.kind === 'moved' && card.fromLocation && (
                  <span class="reconcile-diff-detail">from {card.fromLocation}</span>
                )}
                <button
                  type="button"
                  class="reconcile-diff-dismiss"
                  onClick={() => onDismiss(card.assetTag)}
                  disabled={applying}
                  aria-label={`Cancel this change for ${card.assetTag}`}
                  title="Cancel this change"
                >
                  🗑
                </button>
              </li>
            ))}
          </ul>
        )}

        <div class="modal-actions">
          <button type="button" onClick={onCancel} disabled={applying}>
            Cancel
          </button>
          <button type="button" onClick={onApproveClick} disabled={applying || noChanges}>
            {applying ? 'Applying…' : 'Approve'}
          </button>
        </div>
      </div>
    </div>
  )
}
