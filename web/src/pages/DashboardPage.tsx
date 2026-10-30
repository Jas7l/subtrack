import {
  AppstoreOutlined,
  ClockCircleOutlined,
  PieChartOutlined,
  TeamOutlined,
  WalletOutlined,
} from '@ant-design/icons'
import { Alert, Button, Card, Empty, Spin } from 'antd'
import type { ReactNode } from 'react'
import { Link, useNavigate } from 'react-router'
import { useByCategory, useForecast, useSummary } from '../api/analytics'
import { useReleases } from '../api/releases'
import { useRequests } from '../api/requests'
import { useAuth, useMe } from '../auth/AuthContext'
import { CategoryPie } from '../components/CategoryPie'
import { ForecastChart } from '../components/ForecastChart'
import { PageHeader } from '../components/PageHeader'
import { PrintButton } from '../components/PrintButton'
import { NoDepartment } from '../components/QueryState'
import { ServiceTile } from '../components/ServiceTile'
import { StatCard } from '../components/StatCard'
import { StatusBadge } from '../components/StatusBadge'
import { errorText } from '../lib/errors'
import { formatMoney, fromNow, plural } from '../lib/format'
import { AdminDashboard } from './AdminDashboard'

// One line of the manager's «Ждут вашего решения».
interface InboxItem {
  key: string
  to: string
  tile: string
  title: string
  who: string
  at: string
  extra: ReactNode
}

// DashboardPage: the admin's dashboard for the company, or the manager's
// budget of their department.
export function DashboardPage() {
  const me = useMe()
  return me.role === 'admin' ? <AdminDashboard /> : <ManagerDashboard />
}

// ManagerDashboard: the department's budget.
function ManagerDashboard() {
  const me = useMe()
  const { department, isHead, departmentLoading } = useAuth()
  const navigate = useNavigate()
  const summary = useSummary()
  const categories = useByCategory()
  const forecast = useForecast(12)
  // What waits for the manager: requests for a subscription (the head
  // decides those) and requests to give a licence back (any manager of the
  // department), newest first, the latest five together.
  const inbox = useRequests({ status: 'pending_manager', limit: 5 })
  const releases = useReleases({ inbox: true, limit: 5 })

  if (me.department_id === null) return <NoDepartment title="Бюджет отдела" />
  if (summary.isPending || departmentLoading) {
    return (
      <div className="page-spin">
        <Spin size="large" />
      </div>
    )
  }
  if (summary.error) {
    return <Alert type="error" showIcon title={errorText(summary.error)} />
  }
  const s = summary.data
  const waiting: InboxItem[] = [
    ...(isHead ? (inbox.data?.items ?? []) : []).map((r) => ({
      key: `request-${r.id}`,
      to: `/requests/${r.id}`,
      tile: r.name,
      title: r.name,
      who: r.requester_name,
      at: r.created_at,
      extra: formatMoney(r.estimated_cost, r.currency),
    })),
    ...(releases.data?.items ?? []).map((r) => ({
      key: `release-${r.id}`,
      to: `/release-requests/${r.id}`,
      tile: r.subscription_name,
      title: `Отказ от «${r.subscription_name}»`,
      who: r.requester_name,
      at: r.created_at,
      extra: <StatusBadge tone="warning">отказ</StatusBadge>,
    })),
  ]
    .sort((a, b) => b.at.localeCompare(a.at))
    .slice(0, 5)
  const waitingTotal = (isHead ? (inbox.data?.total ?? 0) : 0) + (releases.data?.total ?? 0)
  const waitingError = (isHead ? inbox.error : null) ?? releases.error
  const waitingLoading = (isHead && inbox.isPending) || releases.isPending

  return (
    <>
      <PageHeader
        title="Бюджет отдела"
        subtitle={department ? `${department.name}: расходы на подписки и лицензии` : undefined}
        extra={<PrintButton />}
      />

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
            s.enterprise_subscriptions > 0
              ? `из них корпоративных: ${s.enterprise_subscriptions}`
              : `${s.licences} ${plural(s.licences, 'лицензия', 'лицензии', 'лицензий')} выдано`
          }
        />
        <StatCard
          icon={<PieChartOutlined />}
          tone="success"
          label="Загрузка мест"
          value={s.utilization_pct === null ? '—' : `${s.utilization_pct}%`}
          hint={`занято ${s.seats_used} из ${s.seats_total}, свободно ${s.seats_free}`}
        />
        <StatCard
          icon={<TeamOutlined />}
          tone="accent"
          label="На сотрудника"
          value={s.cost_per_user_rub === null ? '—' : formatMoney(s.cost_per_user_rub)}
          hint={`в месяц, сотрудников: ${s.active_users}`}
        />
        <StatCard
          icon={<ClockCircleOutlined />}
          tone={s.expiring_soon > 0 ? 'warning' : 'muted'}
          label="Скоро истекают"
          value={s.expiring_soon}
          hint={s.expired > 0 ? `уже истекли: ${s.expired}` : 'истёкших нет'}
        />
      </div>

      <div className="dash-grid">
        <Card
          variant="borderless"
          title={`Ждут вашего решения · ${waitingTotal}`}
          extra={<Link to="/requests">Все заявки</Link>}
        >
          {waitingError ? (
            <Alert type="error" showIcon title={errorText(waitingError)} />
          ) : waiting.length === 0 ? (
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={waitingLoading ? 'Загрузка…' : 'Новых заявок нет'}
            />
          ) : (
            <div className="inbox-list">
              {waiting.map((it) => (
                <button key={it.key} type="button" className="inbox-item" onClick={() => navigate(it.to)}>
                  <ServiceTile name={it.tile} size={36} />
                  <span className="inbox-text">
                    <span className="cell-title">{it.title}</span>
                    <span className="cell-sub">
                      {it.who} · {fromNow(it.at)}
                    </span>
                  </span>
                  <span className="inbox-cost">{it.extra}</span>
                </button>
              ))}
            </div>
          )}
        </Card>
        <Card variant="borderless" title="Расходы по категориям" extra={<span className="cell-sub">в месяц</span>}>
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

      <Card
        variant="borderless"
        title="Прогноз на 12 месяцев"
        extra={
          <Button size="small" onClick={() => navigate('/forecast')}>
            Подробнее
          </Button>
        }
        style={{ marginTop: 24 }}
      >
        {forecast.error ? (
          <Alert type="error" showIcon title={errorText(forecast.error)} />
        ) : forecast.data ? (
          <ForecastChart months={forecast.data.months} height={280} />
        ) : (
          <Spin />
        )}
      </Card>
    </>
  )
}

