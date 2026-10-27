import { onTile, tileColors } from '../theme'

// initials of a service: «Jira Software» → «JS», «Figma» → «FI».
function initials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean)
  if (words.length === 0) return '?'
  if (words.length === 1) return words[0].slice(0, 2).toUpperCase()
  return (words[0][0] + words[1][0]).toUpperCase()
}

// A stable colour per service name, so a service looks the same everywhere.
function colorFor(name: string): string {
  let hash = 0
  for (const ch of name.toLowerCase()) hash = (hash * 31 + ch.charCodeAt(0)) | 0
  return tileColors[Math.abs(hash) % tileColors.length]
}

// ServiceTile is the coloured square with the initials of a service.
export function ServiceTile({ name, size = 48 }: { name: string; size?: number }) {
  return (
    <span
      className="service-tile"
      aria-hidden="true"
      style={{
        width: size,
        height: size,
        fontSize: size * 0.36,
        background: colorFor(name),
        color: onTile,
      }}
    >
      {initials(name)}
    </span>
  )
}
