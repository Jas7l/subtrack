import {
  AppstoreOutlined,
  ClockCircleOutlined,
  PieChartOutlined,
  TeamOutlined,
  WalletOutlined,
} from '@ant-design/icons'
import { Alert, Button, Card, Empty, Spin } from 'antd'
import { Link, useNavigate } from 'react-router'
import { useByCategory, useForecast, useSummary } from '../api/analytics'
import { useRequests } from '../api/requests'
import { useAuth, useMe } from '../auth/AuthContext'
import { CategoryPie } from '../components/CategoryPie'
import { ForecastChart } from '../components/ForecastChart'
import { PageHeader } from '../components/PageHeader'
import { NoDepartment } from '../components/QueryState'
import { ServiceTile } from '../components/ServiceTile'
import { StatCard } from '../components/StatCard'
import { errorText } from '../lib/errors'
import { formatMoney, fromNow, plural } from '../lib/format'

// DashboardPage (manager): the department's budget. The admin's dashboard
// with payments and recommendations is К6.
export function DashboardPage() {
  const me = useMe()
  const { department, isHead, departmentLoading } = useAuth()
  const navigate = useNavigate()
  const summary = useSummary()
  const categories = useByCategory()
  const forecast = useForecast(12)
  const inbox = useRequests({ status: 'pending_manager', limit: 5 })

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

  return (
    <>
      <PageHeader
        title="Бюджет отдела"
        subtitle={department ? `${department.name}: расходы на подписки и лицензии` : undefined}
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
        {isHead && (
          <Card
            variant="borderless"
            title={`Ждут вашего решения · ${inbox.data?.total ?? 0}`}
            extra={<Link to="/requests">Все заявки</Link>}
          >
            {inbox.error ? (
              <Alert type="error" showIcon title={errorText(inbox.error)} />
            ) : (inbox.data?.items.length ?? 0) === 0 ? (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={inbox.isPending ? 'Загрузка…' : 'Новых заявок нет'}
              />
            ) : (
              <div className="inbox-list">
                {inbox.data?.items.map((r) => (
                  <button
                    key={r.id}
                    type="button"
                    className="inbox-item"
                    onClick={() => navigate(`/requests/${r.id}`)}
                  >
                    <ServiceTile name={r.name} size={36} />
                    <span className="inbox-text">
                      <span className="cell-title">{r.name}</span>
                      <span className="cell-sub">
                        {r.requester_name} · {fromNow(r.created_at)}
                      </span>
                    </span>
                    <span className="inbox-cost">{formatMoney(r.estimated_cost, r.currency)}</span>
                  </button>
                ))}
              </div>
            )}
          </Card>
        )}
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

