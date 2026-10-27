import { theme, type ThemeConfig } from 'antd'

// Palette agreed in CLIENT_PLAN.md, section 11 (dark, Discord-like, lime accent).
export const colors = {
  bgLayout: '#1E1F22',
  bgContainer: '#2B2D31',
  bgElevated: '#313338',
  bgInput: '#383A40',
  bgSecondary: '#4E5058',
  border: '#3F4147',
  heading: '#F2F3F5',
  text: '#DBDEE1',
  textMuted: '#B5BAC1',
  textSecondary: '#949BA4',
  accent: '#A3E635',
  accentSoft: 'rgba(163, 230, 53, 0.14)',
  onAccent: '#0C1A12',
  success: '#3DDC97',
  warning: '#F0B232',
  error: '#FF7A7D',
  counter: '#F23F43',
} as const

// Series colours for charts, in order.
export const chartColors = [colors.accent, '#8B93FF', '#F0B232', '#FF8FB1', '#6D7480']

// Colours of avatars and service tiles; the text on them is dark.
export const tileColors = ['#8B93FF', '#F0B232', '#FF8FB1', '#3DDC97', '#A3E635', '#5EC8F2']
export const onTile = '#14152B'

// Soft tone backgrounds (~14%) behind status pills and icons.
export const tones = {
  success: { color: colors.success, bg: 'rgba(61, 220, 151, 0.14)' },
  warning: { color: colors.warning, bg: 'rgba(240, 178, 50, 0.14)' },
  error: { color: colors.error, bg: 'rgba(255, 122, 125, 0.14)' },
  info: { color: '#8B93FF', bg: 'rgba(139, 147, 255, 0.14)' },
  accent: { color: colors.accent, bg: colors.accentSoft },
  muted: { color: colors.textMuted, bg: 'rgba(181, 186, 193, 0.12)' },
} as const

export type Tone = keyof typeof tones

export const themeConfig: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    colorPrimary: colors.accent,
    colorLink: colors.accent,
    colorSuccess: colors.success,
    colorWarning: colors.warning,
    colorError: colors.error,
    colorInfo: '#8B93FF',
    colorBgLayout: colors.bgLayout,
    colorBgContainer: colors.bgContainer,
    colorBgElevated: colors.bgElevated,
    colorBorder: colors.border,
    colorBorderSecondary: colors.border,
    colorSplit: colors.border,
    colorTextHeading: colors.heading,
    colorText: colors.text,
    colorTextSecondary: colors.textSecondary,
    colorTextDescription: colors.textSecondary,
    borderRadius: 10,
    borderRadiusLG: 16,
    controlHeight: 44,
    controlHeightLG: 50,
    fontFamily: "'Manrope Variable', Manrope, system-ui, sans-serif",
    fontSize: 15,
  },
  components: {
    Button: {
      primaryColor: colors.onAccent,
      primaryShadow: 'none',
      defaultBg: colors.bgSecondary,
      defaultColor: colors.heading,
      defaultBorderColor: colors.bgSecondary,
      defaultShadow: 'none',
      fontWeight: 700,
    },
    Input: { colorBgContainer: colors.bgInput, colorBorder: colors.bgInput },
    InputNumber: { colorBgContainer: colors.bgInput, colorBorder: colors.bgInput },
    Select: { colorBgContainer: colors.bgInput, colorBorder: colors.bgInput },
    DatePicker: { colorBgContainer: colors.bgInput, colorBorder: colors.bgInput },
    Badge: { colorError: colors.counter },
    Card: { paddingLG: 24 },
    Menu: {
      itemBg: 'transparent',
      itemColor: colors.textMuted,
      itemHoverColor: colors.heading,
      itemSelectedColor: colors.accent,
      horizontalItemSelectedColor: colors.accent,
      horizontalItemSelectedBg: colors.accentSoft,
      horizontalItemHoverBg: colors.bgInput,
      horizontalItemBorderRadius: 10,
      horizontalLineHeight: '44px',
      activeBarHeight: 0,
      activeBarBorderWidth: 0,
      itemPaddingInline: 14,
      fontWeightStrong: 600,
    },
  },
}
