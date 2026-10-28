import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { DepartmentRowDTO } from '../api/types'
import { axisProps, compactRub, gridProps, toNumber, tooltipProps } from '../lib/charts'
import { formatMoney } from '../lib/format'
import { chartColors } from '../theme'

interface Row {
  name: string
  cost: number
  source: DepartmentRowDTO
}

const ROW_HEIGHT = 44

// DepartmentsChart: the monthly cost of each department, most expensive
// first (the server's order); company-wide subscriptions are «Общие».
export function DepartmentsChart({ items }: { items: DepartmentRowDTO[] }) {
  const data: Row[] = items.map((d) => ({
    name: d.department_id === null ? 'Общие' : d.name,
    cost: toNumber(d.monthly_cost_rub),
    source: d,
  }))
  return (
    <ResponsiveContainer width="100%" height={data.length * ROW_HEIGHT + 32}>
      <BarChart data={data} layout="vertical" margin={{ top: 0, right: 16, bottom: 0, left: 8 }}>
        <CartesianGrid {...gridProps} vertical horizontal={false} />
        <XAxis type="number" tickFormatter={compactRub} {...axisProps} />
        <YAxis type="category" dataKey="name" width={150} {...axisProps} />
        <Tooltip
          {...tooltipProps}
          formatter={(_, __, item) => {
            const d = (item.payload as Row).source
            const seats =
              d.utilization_pct === null ? '' : ` · места ${d.seats_used}/${d.seats_total}`
            return [`${formatMoney(d.monthly_cost_rub)} / мес${seats}`, `${d.subscriptions} подп.`]
          }}
        />
        <Bar
          dataKey="cost"
          fill={chartColors[1]}
          radius={[0, 6, 6, 0]}
          maxBarSize={24}
          isAnimationActive={false}
        />
      </BarChart>
    </ResponsiveContainer>
  )
}
