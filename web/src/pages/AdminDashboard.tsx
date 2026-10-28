import {
  AppstoreOutlined,
  BulbOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  CloseCircleOutlined,
  FileExcelOutlined,
  PieChartOutlined,
  TeamOutlined,
  WalletOutlined,
  WarningOutlined,
} from '@ant-design/icons'
import { Alert, App, Button, Card, Empty, Progress, Spin, Table, type TableProps } from 'antd'
import dayjs from 'dayjs'
import { useState, type ReactNode } from 'react'
import { Link } from 'react-router'
import {
  exportAnalytics,
  useByCategory,
  useByDepartment,
  useRecommendations,
  useSpending,
  useSummary,
  useTopSubscriptions,
} from '../api/analytics'
import { useSubscriptions } from '../api/subscriptions'
import { useTransactions } from '../api/transactions'
import type { SummaryDTO, TopRowDTO } from '../api/types'
import { CategoryPie } from '../components/CategoryPie'
import { DepartmentSelect } from '../components/DepartmentSelect'
import { DepartmentsChart } from '../components/DepartmentsChart'
import { PageHeader } from '../components/PageHeader'
import { PrintButton } from '../components/PrintButton'
import { RecommendationCard } from '../components/RecommendationCard'
import { ServiceTile } from '../components/ServiceTile'
import { SpendingChart } from '../components/SpendingChart'
import { StatCard } from '../components/StatCard'
import { errorText } from '../lib/errors'
import { dashEvents, whenText } from '../lib/events'
import { formatMoney, plural } from '../lib/format'
import { useUrlState } from '../lib/urlState'
import { colors, tones } from '../theme'

// The dashboard reads at most this many subscriptions for its events; a
// company of the thesis has a few dozen.
const ALL_SUBS = 200
const EVENTS = 6

// The current month, the way the payment register filters it (and the
// server counts «Оплачено в этом месяце»: by payment date).
const monthStart = () => dayjs().startOf('month').format('YYYY-MM-DD')
const monthEnd = () => dayjs().endOf('month').format('YYYY-MM-DD')

// AdminDashboard: the company's (or one department's) spending, payments,
// advice and what comes next. The department filter goes into every block;
// the payment figures of the summary follow it too. The breakdown by
// department makes sense only for the whole company.
export function AdminDashboard() {
  const { message } = App.useApp()
  const url = useUrlState()
  const departmentId = Number(url.get('department_id')) || undefined
  const scope = departmentId ? { department_id: departmentId } : {}
  const [exporting, setExporting] = useState(false)

  const summary = useSummary(scope)
  const categories = useByCategory(scope)
  const departments = useByDepartment(!departmentId)
  const spending = useSpending(scope)
  const top = useTopSubscriptions(5, scope)
  const recs = useRecommendations(scope)

  const runExport = () => {
    setExporting(true)
    exportAnalytics(scope)
      .catch((err: unknown) => message.error(errorText(err)))
      .finally(() => setExporting(false))
  }

  return (
    <>
      <PageHeader
        title="Дашборд"
        subtitle="Расходы на подписки, пересчёт в рубли по текущему курсу"
        extra={
          <>
            <DepartmentSelect
              value={departmentId}
              onChange={(id) => url.set({ department_id: id ?? null })}
              placeholder="Все отделы"
              style={{ width: 220 }}
            />
            <Button
              icon={<FileExcelOutlined />}
              loading={exporting}
              onClick={runExport}
              className="no-print"
            >
              Excel
            </Button>
            <PrintButton />
          </>
        }
      />
      {summary.error ? (
        <Alert type="error" showIcon title={errorText(summary.error)} />
      ) : !summary.data ? (
        <div className="page-spin">
          <Spin size="large" />
        </div>
      ) : (
        <>
          <SummaryCards summary={summary.data} />
          <div className="dash-row">
            <Card
              variant="borderless"
              title="Оплачено по месяцам"
              extra={
                spending.data && (
                  <span className="cell-sub">
                    за 12 мес.: {formatMoney(spending.data.total_rub)}
                  </span>
                )
              }
              className="dash-wide"
            >
              {spending.error ? (
                <Alert type="error" showIcon title={errorText(spending.error)} />
              ) : spending.data ? (
                <SpendingChart months={spending.data.months} />
              ) : (
                <Spin />
              )}
            </Card>
            <Card
              variant="borderless"
              title="По категориям"
              extra={<span className="cell-sub">в месяц</span>}
              className="dash-narrow"
            >
              {categories.error ? (
                <Alert type="error" showIcon title={errorText(categories.error)} />
              ) : categories.isPending ? (
                <Spin />
              ) : categories.data.length === 0 ? (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Активных подписок нет" />
              ) : (
                <CategoryPie items={categories.data} />
              )}
            </Card>
          </div>
          <div className="dash-row">
            <Card
              variant="borderless"
              title="Рекомендации"
              extra={
                (recs.data?.items.length ?? 0) > 0 && (
                  <Link to={recsLink(departmentId)}>Все {recs.data?.items.length} →</Link>
                )
              }
              className="dash-half"
            >
              {recs.error ? (
                <Alert type="error" showIcon title={errorText(recs.error)} />
              ) : !recs.data ? (
                <Spin />
              ) : recs.data.items.length === 0 ? (
                <Empty
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                  description="Советов нет: подписки используются полностью"
                />
              ) : (
                <div className="rec-list">
                  {recs.data.items.slice(0, 3).map((r, i) => (
                    <RecommendationCard key={i} item={r} compact />
                  ))}
                </div>
              )}
            </Card>
            <Events departmentId={departmentId} />
          </div>
          <div className="dash-row">
            <Card
              variant="borderless"
              title="Самые дорогие"
              extra={<Link to="/subscriptions">Все подписки</Link>}
              className="dash-wide"
              styles={{ body: { padding: 0 } }}
            >
              {top.error && <Alert type="error" showIcon title={errorText(top.error)} />}
              <Table<TopRowDTO>
                rowKey="subscription_id"
                columns={topColumns}
                dataSource={top.data}
                loading={top.isFetching}
                pagination={false}
                locale={{ emptyText: 'Активных подписок нет' }}
              />
            </Card>
            <FreeSeats departmentId={departmentId} />
          </div>
          {!departmentId && (
            <Card variant="borderless" title="Расходы по отделам" extra={<span className="cell-sub">в месяц</span>}>
              {departments.error ? (
                <Alert type="error" showIcon title={errorText(departments.error)} />
              ) : departments.data ? (
                <DepartmentsChart items={departments.data} />
              ) : (
                <Spin />
              )}
            </Card>
          )}
        </>
      )}
    </>
  )
}

const recsLink = (departmentId?: number) =>
  departmentId ? `/recommendations?department_id=${departmentId}` : '/recommendations'

function SummaryCards({ summary: s }: { summary: SummaryDTO }) {
  const p = s.payments
  const savings = s.potential_savings_rub ?? '0'
  return (
    <>
      <div className="stat-grid">
        <StatCard
          icon={<WalletOutlined />}
          label="Расходы в месяц"
          value={formatMoney(s.monthly_cost_rub)}
          hint={`${formatMoney(s.annual_cost_rub)} в год`}
        />
        <StatCard
          icon={<AppstoreOutlined />}
          tone="info"
          label="Активных подписок"
          value={s.active_subscriptions}
          hint={
            s.expiring_soon > 0
              ? `${s.expiring_soon} скоро ${plural(s.expiring_soon, 'истекает', 'истекают', 'истекают')}`
              : s.expired > 0
                ? `истекли: ${s.expired}`
                : 'истекающих нет'
          }
          to="/subscriptions"
        />
        <StatCard
          icon={<PieChartOutlined />}
          tone="success"
          label="Загрузка мест"
          value={s.utilization_pct === null ? '—' : `${s.utilization_pct}%`}
          hint={
            <>
              <Progress
                percent={s.utilization_pct ?? 0}
                showInfo={false}
                size="small"
                strokeColor={colors.accent}
                railColor={colors.bgLayout}
              />
              занято {s.seats_used} из {s.seats_total}
            </>
          }
        />
        <StatCard
          icon={<TeamOutlined />}
          tone="info"
          label="На сотрудника"
          value={s.cost_per_user_rub === null ? '—' : formatMoney(s.cost_per_user_rub)}
          hint={`в месяц, сотрудников: ${s.active_users}`}
        />
        <StatCard
          icon={<BulbOutlined />}
          tone="accent"
          highlight
          label="Можно сэкономить"
          value={formatMoney(savings)}
          hint={`за год, ${s.recommendations ?? 0} ${plural(s.recommendations ?? 0, 'рекомендация', 'рекомендации', 'рекомендаций')}`}
          to={recsLink(s.department_id ?? undefined)}
        />
      </div>
      {p && (
        <div className="stat-grid">
          <StatCard
            icon={<CheckCircleOutlined />}
            tone="success"
            label="Оплачено в этом месяце"
            value={formatMoney(p.paid_this_month_rub)}
            hint={`${formatMoney(p.paid_this_year_rub)} с начала года`}
            to={`/transactions?tab=all&status=paid&from=${monthStart()}&to=${monthEnd()}`}
          />
          <StatCard
            icon={<ClockCircleOutlined />}
            tone={p.pending_count > 0 ? 'warning' : 'muted'}
            label="Ждут оплаты"
            value={p.pending_count}
            hint={p.pending_count > 0 ? `на ${formatMoney(p.pending_rub)}` : 'всё оплачено'}
            to="/transactions"
          />
          <StatCard
            icon={<WarningOutlined />}
            tone={p.overdue_count > 0 ? 'error' : 'muted'}
            label="Просрочены"
            value={p.overdue_count}
            hint={p.overdue_count > 0 ? 'дата прошла, оплата не отмечена' : 'просрочек нет'}
            to="/transactions"
          />
          <StatCard
            icon={<CloseCircleOutlined />}
            tone={p.failed_count > 0 ? 'error' : 'muted'}
            label="Ошибки оплаты"
            value={p.failed_count}
            hint={p.failed_count > 0 ? 'нужно оплатить повторно' : 'ошибок нет'}
            to="/transactions?tab=all&status=failed"
          />
        </div>
      )}
    </>
  )
}

const topColumns: TableProps<TopRowDTO>['columns'] = [
  {
    title: 'Подписка',
    key: 'name',
    render: (_, t) => (
      <Link to={`/subscriptions/${t.subscription_id}`} className="cell-service">
        <ServiceTile name={t.name} size={32} />
        <div>
          <div className="cell-title">{t.name}</div>
          <div className="cell-sub">{t.department_name ?? 'Общая'}</div>
        </div>
      </Link>
    ),
  },
  {
    title: 'В месяц',
    key: 'cost',
    align: 'right',
    render: (_, t) => <span className="cell-money">{formatMoney(t.monthly_cost_rub)}</span>,
  },
  {
    title: 'Места',
    key: 'seats',
    align: 'right',
    render: (_, t) =>
      t.seats_total === null ? (
        <span className="cell-sub">{t.seats_used} · безлимит</span>
      ) : (
        <span>
          {t.seats_used} / {t.seats_total}
          {t.utilization_pct !== null && (
            <span className="cell-sub"> · {t.utilization_pct}%</span>
          )}
        </span>
      ),
  },
]

// Events: overdue and upcoming payments, expiring and lately expired
// subscriptions. Pending payments carry no department: with a department
// picked they are kept only for its subscriptions.
function Events({ departmentId }: { departmentId?: number }) {
  const subs = useSubscriptions({ department_id: departmentId, limit: ALL_SUBS })
  const pending = useTransactions({ status: 'pending', order: 'asc', limit: ALL_SUBS })
  const ids = new Set((subs.data?.items ?? []).map((s) => s.id))
  const payments = (pending.data?.items ?? []).filter(
    (t) => !departmentId || ids.has(t.subscription_id),
  )
  const events = dashEvents(subs.data?.items ?? [], payments)
  const error = subs.error ?? pending.error

  let body: ReactNode
  if (error) body = <Alert type="error" showIcon title={errorText(error)} />
  else if (subs.isPending || pending.isPending) body = <Spin />
  else if (events.length === 0)
    body = <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Ближайших событий нет" />
  else
    body = (
      <div className="event-list">
        {events.slice(0, EVENTS).map((e) => (
          <Link key={e.key} to={e.to} className="event">
            <span className="event-dot" style={{ background: tones[e.tone].color }} />
            <span className="event-text">{e.text}</span>
            <span className="event-when" style={{ color: tones[e.tone].color }}>
              {whenText(e.days)}
            </span>
          </Link>
        ))}
      </div>
    )

  return (
    <Card
      variant="borderless"
      title="Ближайшие события"
      extra={events.length > EVENTS && <span className="cell-sub">ещё {events.length - EVENTS}</span>}
      className="dash-half"
    >
      {body}
    </Card>
  )
}

// FreeSeats: active subscriptions with seats nobody holds.
function FreeSeats({ departmentId }: { departmentId?: number }) {
  const query = { free_seats: true, status: 'active', department_id: departmentId }
  const subs = useSubscriptions({ ...query, limit: 5 })
  const items = subs.data?.items ?? []
  const all = `/subscriptions?free_seats=true&status=active${departmentId ? `&department_id=${departmentId}` : ''}`
  return (
    <Card
      variant="borderless"
      title={`Свободные места · ${subs.data?.total ?? 0}`}
      extra={(subs.data?.total ?? 0) > items.length && <Link to={all}>Все →</Link>}
      className="dash-narrow"
    >
      {subs.error ? (
        <Alert type="error" showIcon title={errorText(subs.error)} />
      ) : subs.isPending ? (
        <Spin />
      ) : items.length === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Все места заняты" />
      ) : (
        <div className="event-list">
          {items.map((s) => (
            <Link key={s.id} to={`/subscriptions/${s.id}`} className="event">
              <ServiceTile name={s.name} size={28} />
              <span className="event-text">{s.name}</span>
              <span className="event-when">
                свободно {s.seats_free} из {s.seats_total}
              </span>
            </Link>
          ))}
        </div>
      )}
    </Card>
  )
}
