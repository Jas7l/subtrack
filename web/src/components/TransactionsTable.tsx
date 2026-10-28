import { CheckOutlined, CloseOutlined } from '@ant-design/icons'
import { App, Button, Popconfirm, Space, Table, type TableProps } from 'antd'
import { useState } from 'react'
import { Link } from 'react-router'
import { useFailTransaction } from '../api/transactions'
import type { TransactionDTO } from '../api/types'
import { errorText } from '../lib/errors'
import { daysLeft, formatDate, formatMoment, formatMoney, plural } from '../lib/format'
import { isOverdue } from '../lib/transactions'
import { PayTransactionModal } from './PayTransactionModal'
import { ServiceTile } from './ServiceTile'
import { TransactionStatusBadge } from './StatusBadge'

// TransactionsTable: payments with «Оплачено» (pending, failed) and «Ошибка
// оплаты» (pending only). Overdue rows are highlighted. The subscription
// column is left out on the subscription's own card.
export function TransactionsTable({
  items,
  loading,
  pagination,
  emptyText,
  withSubscription = true,
}: {
  items: TransactionDTO[] | undefined
  loading: boolean
  pagination: TableProps<TransactionDTO>['pagination']
  emptyText: string
  withSubscription?: boolean
}) {
  const [paying, setPaying] = useState<TransactionDTO>()

  const columns: TableProps<TransactionDTO>['columns'] = [
    {
      title: 'Дата',
      key: 'date',
      render: (_, t) => {
        const late = -daysLeft(t.payment_date)
        return (
          <div>
            <div>{formatDate(t.payment_date)}</div>
            {isOverdue(t) && (
              <div className="cell-overdue">
                просрочен на {late} {plural(late, 'день', 'дня', 'дней')}
              </div>
            )}
          </div>
        )
      },
    },
    ...(withSubscription
      ? [
          {
            title: 'Подписка',
            key: 'subscription',
            render: (_: unknown, t: TransactionDTO) => (
              <Link to={`/subscriptions/${t.subscription_id}`} className="cell-service">
                <ServiceTile name={t.subscription_name} size={32} />
                <span className="cell-title">{t.subscription_name}</span>
              </Link>
            ),
          },
        ]
      : []),
    {
      title: 'Сумма',
      key: 'amount',
      align: 'right',
      render: (_, t) => <span className="cell-money">{formatMoney(t.amount, t.currency)}</span>,
    },
    {
      title: 'Статус',
      key: 'status',
      render: (_, t) => <TransactionStatusBadge status={t.status} />,
    },
    {
      title: 'Способ · счёт',
      key: 'method',
      render: (_, t) =>
        t.payment_method || t.invoice_number ? (
          <div>
            <div>{t.payment_method || '—'}</div>
            {t.invoice_number && <div className="cell-sub">{t.invoice_number}</div>}
          </div>
        ) : (
          <span className="cell-sub">—</span>
        ),
    },
    {
      title: 'Оплачен',
      key: 'paid',
      render: (_, t) => (t.paid_at ? formatMoment(t.paid_at) : <span className="cell-sub">—</span>),
    },
    {
      key: 'actions',
      align: 'right',
      render: (_, t) =>
        t.status !== 'paid' && (
          <Space>
            <Button
              size="small"
              type="primary"
              icon={<CheckOutlined />}
              onClick={() => setPaying(t)}
            >
              Оплачено
            </Button>
            {t.status === 'pending' && <FailButton transaction={t} />}
          </Space>
        ),
    },
  ]

  return (
    <>
      <Table<TransactionDTO>
        rowKey="id"
        columns={columns}
        dataSource={items}
        loading={loading}
        rowClassName={(t) => (isOverdue(t) ? 'row-overdue' : '')}
        locale={{ emptyText }}
        pagination={pagination}
      />
      <PayTransactionModal transaction={paying} onClose={() => setPaying(undefined)} />
    </>
  )
}

// FailButton: «Ошибка оплаты». The payment can still be paid later, so a
// short confirmation is enough.
function FailButton({ transaction: t }: { transaction: TransactionDTO }) {
  const { message } = App.useApp()
  const fail = useFailTransaction()
  return (
    <Popconfirm
      title="Отметить ошибку оплаты?"
      description="Платёж можно будет оплатить позже."
      okText="Отметить"
      cancelText="Отмена"
      onConfirm={() =>
        fail.mutateAsync(t.id).then(
          () => message.success('Отмечена ошибка оплаты'),
          (err: unknown) => message.error(errorText(err)),
        )
      }
    >
      <Button size="small" danger icon={<CloseOutlined />}>
        Ошибка
      </Button>
    </Popconfirm>
  )
}
