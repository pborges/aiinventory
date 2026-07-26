import type { IconDefinition } from '@fortawesome/fontawesome-svg-core'

interface Props {
  icon: IconDefinition
  class?: string
}

/** Renders a Font Awesome solid icon as inline SVG (no webfont download,
 * no React-specific wrapper needed) — fill inherits currentColor, so icons
 * follow theme/text color same as everything else in the app. */
export function Icon({ icon, class: className }: Props) {
  const [width, height, , , pathData] = icon.icon
  const paths = Array.isArray(pathData) ? pathData : [pathData]

  return (
    <svg
      viewBox={`0 0 ${width} ${height}`}
      width="1em"
      height="1em"
      fill="currentColor"
      aria-hidden="true"
      class={'icon' + (className ? ` ${className}` : '')}
    >
      {paths.map((d, i) => (
        <path key={i} d={d} />
      ))}
    </svg>
  )
}
