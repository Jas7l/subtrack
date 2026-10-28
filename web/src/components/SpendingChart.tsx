import { Bar, BarChart, CartesianGrid, Cell, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { SpendingMonthDTO } from '../api/types'
import {
  axisProps,
  compactRub,
  gridProps,
  monthLabel,
  monthTitle,
  toNumber,
  tooltipProps,
} from '../lib/charts'
import { formatMoney, plural } from '../lib/format'
import { colors } from '../theme'

interface Row {
  month: string
  amount: number
  source: SpendingMonthDTO
}

// SpendingChart: paid payments by month, the current month bright and the
// earlier ones dimmed (as on the mockup).
export function SpendingChart({
  months,
  height = 280,
}: {
  months: SpendingMonthDTO[]
  height?: number
}) {
  const data: Row[] = months.map((m) => ({
    month: m.month,
    amount: toNumber(m.amount_rub),
    source: m,
  }))
  return (
    <ResponsiveContainer width="100%" height={height}>
      <BarChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: 8 }}>
        <CartesianGrid {...gridProps} />
        <XAxis dataKey="month" tickFormatter={monthLabel} {...axisProps} />
        <YAxis tickFormatter={compactRub} width={72} {...axisProps} />
        <Tooltip
          {...tooltipProps}
          labelFormatter={(m) => monthTitle(String(m))}
          formatter={(_, __, item) => {
            const m = (item.payload as Row).source
            return [
              formatMoney(m.amount_rub),
              `${m.payments} ${plural(m.payments, 'платёж', 'платежа', 'платежей')}`,
            ]
          }}
        />
        <Bar dataKey="amount" radius={[6, 6, 0, 0]} maxBarSize={48} isAnimationActive={false}>
          {data.map((d, i) => (
            <Cell
              key={d.month}
              fill={colors.accent}
              fillOpacity={i === data.length - 1 ? 1 : 0.45}
            />
          ))}
        </Bar>
      </BarChart>
    </ResponsiveContainer>
  )
}
