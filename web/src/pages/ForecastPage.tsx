import { CalendarOutlined, RiseOutlined, WarningOutlined } from '@ant-design/icons'
import { Alert, Card, Segmented, Spin, Table, type TableProps } from 'antd'
import { useForecast } from '../api/analytics'
import type { ForecastMonthDTO } from '../api/types'
import { useMe } from '../auth/AuthContext'
import { ForecastChart } from '../components/ForecastChart'
import { PageHeader } from '../components/PageHeader'
import { NoDepartment } from '../components/QueryState'
import { StatCard } from '../components/StatCard'
import { monthTitle } from '../lib/charts'
import { errorText } from '../lib/errors'
import { formatMoney, plural } from '../lib/format'
import { useUrlState } from '../lib/urlState'

const PERIODS = [6, 12, 24]
const DEFAULT_PERIOD = 12

const columns: TableProps<ForecastMonthDTO>['columns'] = [
  {
    title: 'Месяц',
    key: 'month',
    render: (_, m) => <span style={{ textTransform: 'capitalize' }}>{monthTitle(m.month)}</span>,
  },
  {
    title: 'По графику платежей',
    key: 'scheduled',
    align: 'right',
    render: (_, m) => (
      <>
        {formatMoney(m.scheduled_rub)}{' '}
        <span className="cell-sub">
          ({m.payments} {plural(m.payments, 'платёж', 'платежа', 'платежей')})
        </span>
      </>
    ),
  },
  {
    title: 'Оценка',
    key: 'estimated',
    align: 'right',
    render: (_, m) => formatMoney(m.estimated_rub),
  },
  {
    title: 'Итого',
    key: 'total',
    align: 'right',
    render: (_, m) => <span className="cell-title">{formatMoney(m.total_rub)}</span>,
  },
]

// ForecastPage (manager): the department's expected spending by month, from
// the next payment dates and billing periods. The admin's version is К6.
export function ForecastPage() {
  const me = useMe()
  const url = useUrlState()
  const months = PERIODS.includes(Number(url.get('months'))) ? Number(url.get('months')) : DEFAULT_PERIOD
  const forecast = useForecast(months)

  if (me.department_id === null) return <NoDepartment title="Прогноз бюджета" />

  const f = forecast.data
  return (
    <>
      <PageHeader
        title="Прогноз бюджета"
        subtitle="Платежи по подпискам отдела: по датам платежей и периодам оплаты"
        extra={
          <Segmented
            value={months}
            onChange={(v) => url.set({ months: v === DEFAULT_PERIOD ? null : v })}
            options={PERIODS.map((p) => ({ value: p, label: `${p} мес.` }))}
          />
        }
      />
      {forecast.error && (
        <Alert type="error" showIcon title={errorText(forecast.error)} style={{ marginBottom: 24 }} />
      )}
      {!f ? (
        !forecast.error && (
          <div className="page-spin">
            <Spin size="large" />
          </div>
        )
      ) : (
        <>
          <div className="stat-grid">
            <StatCard
              icon={<CalendarOutlined />}
              label={`Всего за ${months} мес.`}
              value={formatMoney(f.total_rub)}
              hint="с учётом дат окончания подписок"
            />
            <StatCard
              icon={<RiseOutlined />}
              tone="info"
              label="Если всё продлевать"
              value={formatMoney(f.run_rate_rub)}
              hint="месячная стоимость × число месяцев"
            />
            {f.unscheduled > 0 && (
              <StatCard
                icon={<WarningOutlined />}
                tone="warning"
                label="Без даты платежа"
                value={f.unscheduled}
                hint={`у ${f.unscheduled} ${plural(f.unscheduled, 'подписки', 'подписок', 'подписок')} нет даты платежа — учтены оценкой`}
              />
            )}
          </div>
          <Card variant="borderless" style={{ marginBottom: 24, opacity: forecast.isPlaceholderData ? 0.6 : 1 }}>
            <ForecastChart months={f.months} />
            <p className="cell-sub" style={{ margin: '12px 0 0' }}>
              Квартальные и годовые подписки оплачиваются разом, поэтому в отдельные месяцы
              бывают всплески.
            </p>
          </Card>
          <Card variant="borderless" styles={{ body: { padding: 0 } }}>
            <Table<ForecastMonthDTO>
              rowKey="month"
              columns={columns}
              dataSource={f.months}
              pagination={false}
              loading={forecast.isFetching}
              summary={() => (
                <Table.Summary.Row>
                  <Table.Summary.Cell index={0}>
                    <b>Итого</b>
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={1} colSpan={3} align="right">
                    <span className="cell-title">{formatMoney(f.total_rub)}</span>
                  </Table.Summary.Cell>
                </Table.Summary.Row>
              )}
            />
          </Card>
        </>
      )}
    </>
  )
}
