import { Bar, BarChart, CartesianGrid, Legend, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { ForecastMonthDTO } from '../api/types'
import {
  axisProps,
  compactRub,
  gridProps,
  monthLabel,
  monthTitle,
  toNumber,
  tooltipProps,
} from '../lib/charts'
import { formatMoney } from '../lib/format'
import { chartColors, colors } from '../theme'

interface Row {
  month: string
  scheduled: number
  estimated: number
  source: ForecastMonthDTO
}

const labels = { scheduled: 'По графику платежей', estimated: 'Оценка (нет даты платежа)' }

// ForecastChart: expected spending per month, the scheduled payments and the
// estimate for subscriptions without a payment date stacked.
export function ForecastChart({ months, height = 320 }: { months: ForecastMonthDTO[]; height?: number }) {
  const data: Row[] = months.map((m) => ({
    month: m.month,
    scheduled: toNumber(m.scheduled_rub),
    estimated: toNumber(m.estimated_rub),
    source: m,
  }))
  const hasEstimate = data.some((d) => d.estimated > 0)

  return (
    <ResponsiveContainer width="100%" height={height}>
      <BarChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: 8 }}>
        <CartesianGrid {...gridProps} />
        <XAxis dataKey="month" tickFormatter={monthLabel} {...axisProps} />
        <YAxis tickFormatter={compactRub} width={72} {...axisProps} />
        <Tooltip
          {...tooltipProps}
          labelFormatter={(m) => monthTitle(String(m))}
          formatter={(_, key, item) => {
            const m = (item.payload as Row).source
            return key === 'scheduled'
              ? [formatMoney(m.scheduled_rub), `${labels.scheduled} (${m.payments})`]
              : [formatMoney(m.estimated_rub), labels.estimated]
          }}
        />
        {hasEstimate && (
          <Legend
            formatter={(key: string) => (
              <span style={{ color: colors.textSecondary }}>{labels[key as keyof typeof labels]}</span>
            )}
          />
        )}
        <Bar
          dataKey="scheduled"
          stackId="total"
          fill={chartColors[0]}
          radius={hasEstimate ? undefined : [6, 6, 0, 0]}
          maxBarSize={48}
        />
        {hasEstimate && (
          <Bar
            dataKey="estimated"
            stackId="total"
            fill={chartColors[1]}
            radius={[6, 6, 0, 0]}
            maxBarSize={48}
          />
        )}
      </BarChart>
    </ResponsiveContainer>
  )
}
