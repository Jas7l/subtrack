import { PlusOutlined, TeamOutlined } from '@ant-design/icons'
import { Alert, Button, Card, Descriptions, Empty, Table, type TableProps } from 'antd'
import { useMemo, useState } from 'react'
import { Link, useParams } from 'react-router'
import { useAssignments } from '../api/assignments'
import { useSubscription } from '../api/subscriptions'
import type { AssignmentDTO, SubscriptionDTO } from '../api/types'
import { useMe } from '../auth/AuthContext'
import { GrantLicenceModal } from '../components/GrantLicenceModal'
import { PageHeader } from '../components/PageHeader'
import { QueryState } from '../components/QueryState'
import { RevokeLicenceButton } from '../components/RevokeLicenceButton'
import { ServiceTile } from '../components/ServiceTile'
import { SubscriptionStatusBadge } from '../components/StatusBadge'
import { UserAvatar } from '../components/UserAvatar'
import { errorText } from '../lib/errors'
import { formatDate, formatDay, plural } from '../lib/format'
import { subscriptionTypeLabels } from '../lib/labels'
import { canManageSubscription } from '../lib/permissions'
import { costText, isOpen, seatsText } from '../lib/subscriptions'

// SubscriptionPage (manager, employee): the subscription and its licences.
// Holders are listed only to whoever manages the subscription: for anyone
// else the server returns just their own licence. The admin's card is К4.
export function SubscriptionPage() {
  const id = Number(useParams().id)
  const me = useMe()
  const query = useSubscription(id)

  if (!query.data) {
    return (
      <QueryState error={query.error} missing={!Number.isInteger(id)} notFound="Подписка не найдена" />
    )
  }
  const s = query.data
  const manages = canManageSubscription(me, s)

  return (
    <>
      <PageHeader
        title={
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 16 }}>
            <ServiceTile name={s.name} size={52} />
            {s.name}
          </span>
        }
        subtitle={
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 12 }}>
            <SubscriptionStatusBadge status={s.status} />
            {[s.vendor, s.category].filter(Boolean).join(' · ')}
          </span>
        }
      />
      <div className="two-columns">
        <Card variant="borderless" title="Подписка">
          <Descriptions
            column={1}
            colon={false}
            items={[
              { key: 'vendor', label: 'Вендор', children: s.vendor || '—' },
              { key: 'category', label: 'Категория', children: s.category || '—' },
              { key: 'type', label: 'Тип', children: subscriptionTypeLabels[s.type] },
              { key: 'cost', label: 'Стоимость', children: costText(s) },
              { key: 'seats', label: 'Места', children: seatsText(s) },
              { key: 'start', label: 'Начало', children: formatDate(s.start_date) },
              {
                key: 'end',
                label: 'Окончание',
                children: s.end_date ? formatDate(s.end_date) : 'Бессрочно',
              },
              {
                key: 'payment',
                label: 'Следующий платёж',
                children: formatDate(s.next_payment_date),
              },
              { key: 'department', label: 'Отдел', children: s.department_name ?? 'Общая' },
            ]}
          />
        </Card>
        {manages ? <Holders subscription={s} /> : <MyLicence subscription={s} />}
      </div>
    </>
  )
}

// A department has far fewer than 200 holders of one subscription.
const ALL = { limit: 200 }

function Holders({ subscription: s }: { subscription: SubscriptionDTO }) {
  const [granting, setGranting] = useState(false)
  const assignments = useAssignments({ subscription_id: s.id, status: 'active', ...ALL })
  const items = useMemo(() => assignments.data?.items ?? [], [assignments.data])
  const holders = useMemo(() => new Set(items.map((a) => a.user_id)), [items])
  const noSeats = s.seats_free === 0

  const columns: TableProps<AssignmentDTO>['columns'] = [
    {
      title: 'Сотрудник',
      key: 'user',
      render: (_, a) => (
        <Link to={`/users/${a.user_id}`} className="cell-service">
          <UserAvatar id={a.user_id} name={a.user_name} size={32} />
          <span className="cell-title">{a.user_name}</span>
        </Link>
      ),
    },
    { title: 'Выдана', key: 'assigned', render: (_, a) => formatDay(a.assigned_at) },
    { key: 'actions', align: 'right', render: (_, a) => <RevokeLicenceButton assignment={a} /> },
  ]

  return (
    <Card
      variant="borderless"
      title={`Кто пользуется · ${items.length}`}
      extra={
        isOpen(s) && (
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 12 }}>
            {noSeats && <span className="cell-sub">мест нет</span>}
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setGranting(true)}>
              Выдать лицензию
            </Button>
          </span>
        )
      }
      styles={{ body: { padding: items.length ? 0 : undefined } }}
    >
      {assignments.error && <Alert type="error" showIcon title={errorText(assignments.error)} />}
      {items.length === 0 && !assignments.isPending ? (
        <Empty
          image={<TeamOutlined style={{ fontSize: 40, color: '#949BA4' }} />}
          description="Лицензии пока никому не выданы"
        />
      ) : (
        <Table<AssignmentDTO>
          rowKey="id"
          columns={columns}
          dataSource={items}
          loading={assignments.isFetching}
          pagination={false}
        />
      )}
      <GrantLicenceModal
        open={granting}
        onClose={() => setGranting(false)}
        subscription={s}
        holders={holders}
      />
    </Card>
  )
}

function MyLicence({ subscription: s }: { subscription: SubscriptionDTO }) {
  const me = useMe()
  const mine = useAssignments({ subscription_id: s.id, user_id: me.id, status: 'active', limit: 1 })
  const licence = mine.data?.items[0]
  const holders = s.seats_used

  return (
    <Card variant="borderless" title="Ваша лицензия">
      {mine.error ? (
        <Alert type="error" showIcon title={errorText(mine.error)} />
      ) : mine.isPending ? null : licence ? (
        <p style={{ margin: 0 }}>
          У вас есть лицензия с {formatDay(licence.assigned_at)}. Всего ей пользуются {holders}{' '}
          {plural(holders, 'человек', 'человека', 'человек')}.
        </p>
      ) : (
        <p style={{ margin: 0 }}>У вас нет лицензии этой подписки.</p>
      )}
    </Card>
  )
}
