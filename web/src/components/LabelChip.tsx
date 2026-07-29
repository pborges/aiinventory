import { faXmark } from '@fortawesome/free-solid-svg-icons'
import type { Label } from '../api/client'
import { Icon } from './Icon'

interface Props {
  label: Label
  selected?: boolean
  onClick?: () => void
  onRemove?: () => void
}

/** A colored pill for one label, driven by --label-color (see index.css's
 * .label-chip rules) so every rendering context — cards, toggle-clouds,
 * the Settings list — shares one color-mix-based look. */
export function LabelChip({ label, selected, onClick, onRemove }: Props) {
  const interactive = !!onClick
  const As = interactive ? 'button' : 'span'

  return (
    <As
      type={interactive ? 'button' : undefined}
      class={`label-chip${selected === false ? ' label-chip-unselected' : ''}${selected ? ' label-chip-selected' : ''}`}
      style={{ '--label-color': label.color }}
      onClick={onClick}
    >
      {label.name}
      {onRemove && (
        <button
          type="button"
          class="label-chip-remove"
          aria-label={`Remove ${label.name}`}
          onClick={(e) => {
            e.stopPropagation()
            onRemove()
          }}
        >
          <Icon icon={faXmark} />
        </button>
      )}
    </As>
  )
}
