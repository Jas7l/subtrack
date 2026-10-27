import { PlusOutlined } from '@ant-design/icons'
import { Alert, App, Button, Card, Empty, Popconfirm, Spin } from 'antd'
import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router'
import { useAssignments, useRevokeAssignment } from '../api/assignments'
import { useSubscriptions } from '../api/subscriptions'
import type { AssignmentDTO, SubscriptionDTO } from '../api/types'
import { useMe } from '../auth/AuthContext'
import { NewRequestModal } from '../components/NewRequestModal'
import { PageHeader } from '../components/PageHeader'
import { ServiceTile } from '../components/ServiceTile'
import { SubscriptionStatusBadge } from '../components/StatusBadge'
import { errorText } from '../lib/errors'
import { daysLeft, formatDate, formatDay, plural } from '../lib/format'

// The server caps a page at 200; nobody holds that many licences.
const ALL = { limit: 200 }

export function MySubscriptionsPage() {
  const me = useMe()
  const navigate = useNavigate()
  const [asking, setAsking] = useState(false)

  // Licences give the list; subscriptions add the vendor, dates and status.
  const assignments = useAssignments({ user_id: me.id, status: 'active', ...ALL })
  const subscriptions = useSubscriptions(ALL)
  const byId = useMemo(
    () => new Map((subscriptions.data?.items ?? []).map((s) => [s.id, s])),
    [subscriptions.data],
  )

  const items = assignments.data?.items ?? []
  const loading = assignments.isPending || subscriptions.isPending
  const error = assignments.error ?? subscriptions.error

  const ask = (
    <Button type="primary" icon={<PlusOutlined />} onClick={() => setAsking(true)}>
      Запросить подписку
    </Button>
  )

  return (
    <>
      <PageHeader
        title="Мои подписки"
        subtitle={
          items.length > 0
            ? `Вам выдано: ${items.length} ${plural(items.length, 'лицензия', 'лицензии', 'лицензий')}`
            : 'Сервисы, лицензии которых выданы вам'
        }
        extra={ask}
      />
      {error && <Alert type="error" showIcon title={errorText(error)} style={{ marginBottom: 24 }} />}
      {loading ? (
        <div className="page-spin">
          <Spin size="large" />
        </div>
      ) : items.length === 0 ? (
        <Card variant="borderless">
          <Empty description="У вас пока нет подписок. Нужен сервис — отправьте заявку.">
            {ask}
          </Empty>
        </Card>
      ) : (
        <div className="card-grid">
          {items.map((a) => (
            <SubscriptionCard key={a.id} assignment={a} subscription={byId.get(a.subscription_id)} />
          ))}
        </div>
      )}
      <NewRequestModal
        open={asking}
        onClose={() => setAsking(false)}
        onCreated={(req) => navigate(`/requests/${req.id}`)}
      />
    </>
  )
}

function SubscriptionCard({
  assignment,
  subscription: sub,
}: {
  assignment: AssignmentDTO
  subscription: SubscriptionDTO | undefined
}) {
  const { message } = App.useApp()
  const revoke = useRevokeAssignment()
  const expiring = sub?.status === 'expiring_soon'

  const onRevoke = () =>
    revoke.mutateAsync(assignment.id).then(
      () => message.success(`Вы отказались от «${assignment.subscription_name}»`),
      (err: unknown) => message.error(errorText(err)),
    )

  return (
    <Card variant="borderless" className={`sub-card${expiring ? ' sub-card-expiring' : ''}`}>
      <div className="sub-card-head">
        <ServiceTile name={assignment.subscription_name} />
        <div style={{ minWidth: 0, flex: 1 }}>
          <div className="sub-card-name">{assignment.subscription_name}</div>
          <div className="sub-card-vendor">{sub?.vendor || '—'}</div>
        </div>
        {sub && <SubscriptionStatusBadge status={sub.status} />}
      </div>
      <dl className="sub-card-facts">
        <div>
          <dt>Категория</dt>
          <dd>{sub?.category || '—'}</dd>
        </div>
        <div>
          <dt>Действует до</dt>
          <dd>{sub ? <EndDate date={sub.end_date} /> : '—'}</dd>
        </div>
        <div>
          <dt>Выдана</dt>
          <dd>{formatDay(assignment.assigned_at)}</dd>
        </div>
      </dl>
      <div className="sub-card-actions">
        <Popconfirm
          title="Отказаться от подписки?"
          description="Лицензия освободится для коллег. Вернуть её можно только новой заявкой."
          okText="Отказаться"
          cancelText="Оставить"
          okButtonProps={{ danger: true }}
          onConfirm={onRevoke}
        >
          <Button danger loading={revoke.isPending}>
            Отказаться
          </Button>
        </Popconfirm>
      </div>
    </Card>
  )
}

function EndDate({ date }: { date: string | null }) {
  if (!date) return <>Бессрочно</>
  const left = daysLeft(date)
  if (left < 0 || left > 30) return <>{formatDate(date)}</>
  return (
    <>
      {formatDate(date)}{' '}
      <span style={{ color: '#F0B232' }}>
        ({left === 0 ? 'сегодня' : `${left} ${plural(left, 'день', 'дня', 'дней')}`})
      </span>
    </>
  )
}
