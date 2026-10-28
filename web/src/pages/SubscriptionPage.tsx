import {
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
  StopOutlined,
  TeamOutlined,
} from '@ant-design/icons'
import { Alert, App, Button, Card, Descriptions, Empty, Table, type TableProps } from 'antd'
import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { useAssignments } from '../api/assignments'
import {
  useDeleteSubscription,
  useSubscription,
  useUpdateSubscription,
} from '../api/subscriptions'
import { useTransactions } from '../api/transactions'
import type { AssignmentDTO, SubscriptionDTO } from '../api/types'
import { useMe } from '../auth/AuthContext'
import { GrantLicenceModal } from '../components/GrantLicenceModal'
import { PageHeader } from '../components/PageHeader'
import { QueryState } from '../components/QueryState'
import { RevokeLicenceButton } from '../components/RevokeLicenceButton'
import { ServiceTile } from '../components/ServiceTile'
import { SubscriptionFormModal } from '../components/SubscriptionFormModal'
import { TransactionFormModal } from '../components/TransactionFormModal'
import { TransactionsTable } from '../components/TransactionsTable'
import { SubscriptionStatusBadge } from '../components/StatusBadge'
import { UserAvatar } from '../components/UserAvatar'
import { errorText } from '../lib/errors'
import { formatDate, formatDay, plural } from '../lib/format'
import { subscriptionTypeLabels } from '../lib/labels'
import { canManageSubscription } from '../lib/permissions'
import { costText, isOpen, seatsText } from '../lib/subscriptions'

// SubscriptionPage: the subscription and its licences. Holders are listed
// only to whoever manages the subscription (the admin, a manager of the
// owning department): for anyone else the server returns just their own
// licence. The admin also edits, cancels and deletes it and sees its
// payments with «Добавить платёж»; the change history from the audit (К7)
// goes under them.
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
        extra={me.role === 'admin' && <AdminActions subscription={s} />}
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
      {me.role === 'admin' && <Payments subscription={s} />}
    </>
  )
}

// AdminActions: «Редактировать», «Отменить подписку» and «Удалить». Both
// cancelling and deleting revoke every licence, so they ask first.
function AdminActions({ subscription: s }: { subscription: SubscriptionDTO }) {
  const { message, modal } = App.useApp()
  const navigate = useNavigate()
  const [editing, setEditing] = useState(false)
  const update = useUpdateSubscription()
  const remove = useDeleteSubscription()
  const licences =
    s.seats_used > 0
      ? `Все выданные лицензии (${s.seats_used}) будут отозваны.`
      : 'Лицензий по ней не выдано.'

  const cancel = () =>
    modal.confirm({
      title: `Отменить подписку «${s.name}»?`,
      content: `${licences} Вернуть её можно, сменив статус в редактировании, но лицензии придётся выдать заново.`,
      okText: 'Отменить подписку',
      okButtonProps: { danger: true },
      cancelText: 'Не отменять',
      onOk: () =>
        update.mutateAsync({ id: s.id, body: { status: 'cancelled' } }).then(
          () => message.success('Подписка отменена'),
          (err: unknown) => message.error(errorText(err)),
        ),
    })

  const del = () =>
    modal.confirm({
      title: `Удалить подписку «${s.name}»?`,
      content: `${licences} Подписка пропадёт из всех списков; история платежей и аудит останутся.`,
      okText: 'Удалить',
      okButtonProps: { danger: true },
      cancelText: 'Оставить',
      onOk: () =>
        remove.mutateAsync(s.id).then(
          () => {
            message.success('Подписка удалена')
            navigate('/subscriptions', { replace: true })
          },
          (err: unknown) => message.error(errorText(err)),
        ),
    })

  return (
    <>
      <Button icon={<EditOutlined />} onClick={() => setEditing(true)}>
        Редактировать
      </Button>
      {s.status !== 'cancelled' && (
        <Button icon={<StopOutlined />} onClick={cancel}>
          Отменить подписку
        </Button>
      )}
      <Button danger icon={<DeleteOutlined />} onClick={del}>
        Удалить
      </Button>
      <SubscriptionFormModal open={editing} onClose={() => setEditing(false)} subscription={s} />
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

const PAYMENTS_PAGE = 10

// Payments: the subscription's payment history, newest first, with the
// same «Оплачено» / «Ошибка» as the register. The admin only.
function Payments({ subscription: s }: { subscription: SubscriptionDTO }) {
  const [page, setPage] = useState(1)
  const [adding, setAdding] = useState(false)
  const payments = useTransactions({
    subscription_id: s.id,
    limit: PAYMENTS_PAGE,
    offset: (page - 1) * PAYMENTS_PAGE,
  })
  const total = payments.data?.total ?? 0

  return (
    <Card
      variant="borderless"
      title={`История платежей · ${total}`}
      style={{ marginTop: 24 }}
      extra={
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 16 }}>
          {total > 0 && (
            <Link to={`/transactions?tab=all&subscription_id=${s.id}`}>В реестре платежей</Link>
          )}
          <Button icon={<PlusOutlined />} onClick={() => setAdding(true)}>
            Добавить платёж
          </Button>
        </span>
      }
      styles={{ body: { padding: 0 } }}
    >
      {payments.error && <Alert type="error" showIcon title={errorText(payments.error)} />}
      <TransactionsTable
        items={payments.data?.items}
        loading={payments.isFetching}
        withSubscription={false}
        emptyText="Платежей пока нет"
        pagination={{
          current: page,
          pageSize: PAYMENTS_PAGE,
          total,
          hideOnSinglePage: true,
          onChange: setPage,
        }}
      />
      <TransactionFormModal open={adding} onClose={() => setAdding(false)} subscription={s} />
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
