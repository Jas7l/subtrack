import { Avatar } from 'antd'

const palette = ['#8B93FF', '#F0B232', '#FF8FB1', '#3DDC97', '#A3E635', '#5EC8F2']

// initials: «Иванов Иван Сергеевич» → «ИИ» (surname and first name).
function initials(fullName: string): string {
  const parts = fullName.trim().split(/\s+/).filter(Boolean)
  return parts
    .slice(0, 2)
    .map((p) => p[0].toUpperCase())
    .join('')
}

// A stable colour per person, so the same user looks the same everywhere.
function colorFor(id: number): string {
  return palette[Math.abs(id) % palette.length]
}

export function UserAvatar({ id, name, size = 40 }: { id: number; name: string; size?: number }) {
  return (
    <Avatar
      size={size}
      style={{
        flex: 'none',
        background: colorFor(id),
        color: '#14152B',
        fontWeight: 800,
        fontSize: size * 0.38,
      }}
    >
      {initials(name)}
    </Avatar>
  )
}
