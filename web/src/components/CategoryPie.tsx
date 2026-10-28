import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from 'recharts'
import type { SliceDTO } from '../api/types'
import { pieSlices, tooltipProps, type PieSlice } from '../lib/charts'
import { formatMoney } from '../lib/format'
import { colors } from '../theme'

// CategoryPie: the monthly cost by category, a donut with its legend.
export function CategoryPie({ items }: { items: SliceDTO[] }) {
  const slices = pieSlices(items)
  return (
    <div className="pie">
      <div className="pie-chart">
        <ResponsiveContainer width="100%" height={220}>
          <PieChart>
            <Pie
              data={slices}
              dataKey="share"
              nameKey="name"
              innerRadius={62}
              outerRadius={100}
              paddingAngle={2}
              stroke={colors.bgContainer}
              isAnimationActive={false}
            >
              {slices.map((s) => (
                <Cell key={s.name} fill={s.color} />
              ))}
            </Pie>
            <Tooltip
              {...tooltipProps}
              formatter={(_, name, item) => {
                const s = item.payload as PieSlice
                return [s.amount ? `${formatMoney(s.amount)} / мес · ${s.share}%` : `${s.share}%`, name]
              }}
            />
          </PieChart>
        </ResponsiveContainer>
      </div>
      <ul className="pie-legend">
        {slices.map((s) => (
          <li key={s.name}>
            <span className="pie-dot" style={{ background: s.color }} />
            <span className="pie-name">{s.name}</span>
            <span className="pie-value">
              {s.amount ? formatMoney(s.amount) : `${s.subscriptions} подп.`}
            </span>
            <span className="pie-share">{s.share}%</span>
          </li>
        ))}
      </ul>
    </div>
  )
}
