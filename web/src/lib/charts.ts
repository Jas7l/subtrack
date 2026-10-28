import dayjs from 'dayjs'
import type { SliceDTO } from '../api/types'
import { chartColors, colors } from '../theme'

// Recharts defaults are for a light page: a white tooltip, grey #666 axes.
// These props make charts fit the dark theme.
export const axisProps = {
  tick: { fill: colors.textSecondary, fontSize: 13 },
  axisLine: { stroke: colors.border },
  tickLine: { stroke: colors.border },
}

export const gridProps = { stroke: colors.border, strokeDasharray: '3 3', vertical: false }

export const tooltipProps = {
  contentStyle: {
    background: colors.bgElevated,
    border: `1px solid ${colors.border}`,
    borderRadius: 10,
  },
  labelStyle: { color: colors.heading, fontWeight: 700 },
  itemStyle: { color: colors.text },
  cursor: { fill: 'rgba(255, 255, 255, 0.04)' },
}

// Chart heights need numbers: server amounts (strings) become numbers only for
// the geometry; labels and tooltips show the server's strings.
export const toNumber = (amount: string) => Number(amount) || 0

const compact = new Intl.NumberFormat('ru-RU', { notation: 'compact', maximumFractionDigits: 1 })
export const compactRub = (n: number) => `${compact.format(n)} ₽`

// monthLabel: "2026-11" → «ноя 26».
export const monthLabel = (month: string) => dayjs(month, 'YYYY-MM').format('MMM YY')
export const monthTitle = (month: string) => dayjs(month, 'YYYY-MM').format('MMMM YYYY')

// isCurrentMonth: in a forecast, the current month is what is left of it —
// the payment dates already passed have become payments in the register.
export const isCurrentMonth = (month: string) => month === dayjs().format('YYYY-MM')

export interface PieSlice {
  name: string
  share: number
  // null for «Прочие»: their money would have to be added up here.
  amount: string | null
  subscriptions: number
  color: string
}

// pieSlices keeps the four biggest categories (the server sorts them) and
// folds the rest into «Прочие», so five colours are enough.
export function pieSlices(items: SliceDTO[]): PieSlice[] {
  const named = (s: SliceDTO, i: number): PieSlice => ({
    name: s.key || 'Без категории',
    share: s.share_pct,
    amount: s.monthly_cost_rub,
    subscriptions: s.subscriptions,
    color: chartColors[i],
  })
  if (items.length <= chartColors.length) return items.map(named)
  const head = items.slice(0, chartColors.length - 1).map(named)
  const rest = items.slice(chartColors.length - 1)
  head.push({
    name: 'Прочие',
    share: Math.round(rest.reduce((sum, s) => sum + s.share_pct, 0) * 10) / 10,
    amount: null,
    subscriptions: rest.reduce((sum, s) => sum + s.subscriptions, 0),
    color: chartColors[chartColors.length - 1],
  })
  return head
}
