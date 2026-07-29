interface Props {
  value: string
  onChange: (color: string) => void
}

// The 5 Monokai accents already wired into index.css's :root palette, offered
// as one-click presets; the native color input covers everything else.
const MONOKAI_SWATCHES = [
  { name: 'green', color: '#a6e22e' },
  { name: 'pink', color: '#f92672' },
  { name: 'blue', color: '#66d9ef' },
  { name: 'orange', color: '#fd971f' },
  { name: 'purple', color: '#ae81ff' },
]

export function LabelColorPicker({ value, onChange }: Props) {
  return (
    <div class="label-color-picker">
      {MONOKAI_SWATCHES.map((swatch) => (
        <button
          type="button"
          key={swatch.color}
          class={`label-color-swatch${value.toLowerCase() === swatch.color ? ' label-color-swatch-selected' : ''}`}
          style={{ '--swatch-color': swatch.color }}
          aria-label={swatch.name}
          onClick={() => onChange(swatch.color)}
        />
      ))}
      <input
        type="color"
        class="label-color-custom"
        value={value || '#a6e22e'}
        onInput={(e) => onChange((e.target as HTMLInputElement).value)}
        aria-label="Custom color"
      />
    </div>
  )
}
