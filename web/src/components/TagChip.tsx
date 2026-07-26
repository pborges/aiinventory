import { faXmark } from '@fortawesome/free-solid-svg-icons'
import type { Tag } from '../api/client'
import { Icon } from './Icon'

interface Props {
  tag: Tag
  selected?: boolean
  onClick?: () => void
  onRemove?: () => void
}

/** A colored pill for one tag, driven by --tag-color (see index.css's
 * .tag-chip rules) so every rendering context — cards, toggle-clouds,
 * the Settings list — shares one color-mix-based look. */
export function TagChip({ tag, selected, onClick, onRemove }: Props) {
  const interactive = !!onClick
  const As = interactive ? 'button' : 'span'

  return (
    <As
      type={interactive ? 'button' : undefined}
      class={`tag-chip${selected === false ? ' tag-chip-unselected' : ''}${selected ? ' tag-chip-selected' : ''}`}
      style={{ '--tag-color': tag.color }}
      onClick={onClick}
    >
      {tag.name}
      {onRemove && (
        <button
          type="button"
          class="tag-chip-remove"
          aria-label={`Remove ${tag.name}`}
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
