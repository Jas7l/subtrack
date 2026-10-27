import { colors } from '../theme'

// Logo «Продление»: an S with a renewal arrow, lowercase «subtrack».
export function Logo({ size = 36, text = true }: { size?: number; text?: boolean }) {
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: size / 3 }}>
      <svg width={size} height={size} viewBox="0 0 44 44" aria-hidden="true">
        <rect width="44" height="44" rx="22" fill={colors.bgInput} />
        <path
          d="M29 15.5c-1.6-1.9-4-3-7-3-4.2 0-7 2.2-7 5.2 0 6.6 14 3.6 14 10.3 0 3-2.9 5.5-7.2 5.5-3.2 0-5.9-1.3-7.5-3.5"
          fill="none"
          stroke={colors.accent}
          strokeWidth="3.2"
          strokeLinecap="round"
        />
        <path
          d="M26 11.5l3.4 4.2-4.6 1.6"
          fill="none"
          stroke={colors.accent}
          strokeWidth="3"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
      {text && (
        <span style={{ fontSize: size * 0.6, fontWeight: 800, color: colors.heading, lineHeight: 1 }}>
          sub<span style={{ color: colors.accent }}>track</span>
        </span>
      )}
    </span>
  )
}
