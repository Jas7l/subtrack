import { BulbOutlined } from '@ant-design/icons'
import { Alert, Card, Empty, Spin } from 'antd'
import { useRecommendations } from '../api/analytics'
import { DepartmentSelect } from '../components/DepartmentSelect'
import { PageHeader } from '../components/PageHeader'
import { PrintButton } from '../components/PrintButton'
import { RecommendationCard } from '../components/RecommendationCard'
import { errorText } from '../lib/errors'
import { formatMoney, plural } from '../lib/format'
import { useUrlState } from '../lib/urlState'
import { tones } from '../theme'

// RecommendationsPage (admin): the server's advice on cutting costs, the
// biggest yearly saving first; advice whose saving cannot be computed comes
// last. Each links to its subscriptions, where seats are cut or the
// subscription cancelled.
export function RecommendationsPage() {
  const url = useUrlState()
  const departmentId = Number(url.get('department_id')) || undefined
  const recs = useRecommendations(departmentId ? { department_id: departmentId } : {})
  const items = recs.data?.items ?? []

  return (
    <>
      <PageHeader
        title="Рекомендации"
        subtitle="Где подписки используются не полностью и как на этом сэкономить"
        extra={
          <>
            <DepartmentSelect
              value={departmentId}
              onChange={(id) => url.set({ department_id: id ?? null })}
              placeholder="Все отделы"
              style={{ width: 220 }}
            />
            <PrintButton />
          </>
        }
      />
      {recs.error ? (
        <Alert type="error" showIcon title={errorText(recs.error)} />
      ) : !recs.data ? (
        <div className="page-spin">
          <Spin size="large" />
        </div>
      ) : (
        <>
          <Card
            variant="borderless"
            className="savings-banner"
            style={{ background: tones.accent.bg, marginBottom: 24 }}
          >
            <span className="stat-icon" style={{ color: tones.accent.color, marginBottom: 0 }}>
              <BulbOutlined />
            </span>
            <div>
              <div className="stat-label">Потенциальная экономия</div>
              <div className="stat-value" style={{ color: tones.accent.color }}>
                {formatMoney(recs.data.total_savings_rub_year)} в год
              </div>
            </div>
            <div className="cell-sub" style={{ marginLeft: 'auto' }}>
              {items.length} {plural(items.length, 'совет', 'совета', 'советов')}
            </div>
          </Card>
          {items.length === 0 ? (
            <Card variant="borderless">
              <Empty description="Советов нет: подписки используются полностью" />
            </Card>
          ) : (
            <Card
              variant="borderless"
              style={{ opacity: recs.isPlaceholderData ? 0.6 : 1 }}
            >
              <div className="rec-list">
                {items.map((r, i) => (
                  <RecommendationCard key={i} item={r} />
                ))}
              </div>
            </Card>
          )}
        </>
      )}
    </>
  )
}
