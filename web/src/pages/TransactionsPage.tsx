import { FileExcelOutlined, PlusOutlined } from '@ant-design/icons'
import { Alert, App, Badge, Button, Card, DatePicker, Select, Tabs } from 'antd'
import dayjs from 'dayjs'
import { useState } from 'react'
import { exportTransactions, useTransactions } from '../api/transactions'
import type { TransactionStatus } from '../api/types'
import { PageHeader } from '../components/PageHeader'
import { SubscriptionSelect } from '../components/SubscriptionSelect'
import { TransactionFormModal } from '../components/TransactionFormModal'
import { TransactionsTable } from '../components/TransactionsTable'
import { errorText } from '../lib/errors'
import { parseDate, plural } from '../lib/format'
import { transactionStatusLabels } from '../lib/labels'
import { useUrlState } from '../lib/urlState'

const statusOptions = (Object.keys(transactionStatusLabels) as TransactionStatus[]).map((s) => ({
  value: s,
  label: transactionStatusLabels[s],
}))

// Expected payments: pending ones, the oldest (overdue) first.
const EXPECTED = { status: 'pending', order: 'asc' }

// TransactionsPage (admin): the payment register. «Ожидаемые» — pending
// payments the scheduler created on next_payment_date, overdue ones
// highlighted; «Все» — every payment with filters by subscription, status
// and period. Payments are marked paid or failed in the row; one-off and
// past payments are entered by hand. The Excel file takes the tab's filters.
export function TransactionsPage() {
  const { message } = App.useApp()
  const url = useUrlState()
  const [adding, setAdding] = useState(false)
  const [exporting, setExporting] = useState(false)
  const tab = url.get('tab') === 'all' ? 'all' : 'expected'
  const filters = {
    subscription_id: url.get('subscription_id'),
    status: url.get('status'),
    from: url.get('from'),
    to: url.get('to'),
  }
  const filtered = Object.values(filters).some(Boolean)
  const query = tab === 'expected' ? EXPECTED : filters
  const transactions = useTransactions({
    ...query,
    limit: url.pageSize,
    offset: url.offset,
  })
  // The counters do not depend on the page: only `total` is read.
  const pending = useTransactions({ status: 'pending', limit: 1 })
  const yesterday = dayjs().subtract(1, 'day').format('YYYY-MM-DD')
  const overdue = useTransactions({
    status: 'pending',
    to: yesterday,
    limit: 1,
  })
  const overdueCount = overdue.data?.total ?? 0

  const runExport = () => {
    setExporting(true)
    exportTransactions(query)
      .catch((err: unknown) => message.error(errorText(err)))
      .finally(() => setExporting(false))
  }

  const table = (
    <>
      {tab === 'expected' && overdueCount > 0 && (
        <Alert
          type="warning"
          showIcon
          title={`Просрочено ${overdueCount} ${plural(overdueCount, 'платёж', 'платежа', 'платежей')}: дата уже прошла, а оплата не отмечена`}
          style={{ marginBottom: 16 }}
        />
      )}
      {tab === 'all' && (
        <div className="filters">
          <SubscriptionSelect
            allowClear
            value={Number(filters.subscription_id) || undefined}
            onChange={(id) => url.set({ subscription_id: id ?? null })}
            placeholder="Любая подписка"
            style={{ width: 280 }}
          />
          <Select
            allowClear
            placeholder="Любой статус"
            value={filters.status || undefined}
            options={statusOptions}
            onChange={(v?: string) => url.set({ status: v ?? null })}
            style={{ width: 180 }}
          />
          <DatePicker.RangePicker
            allowEmpty={[true, true]}
            format="DD.MM.YYYY"
            placeholder={['Дата с', 'по']}
            value={[
              filters.from ? parseDate(filters.from) : null,
              filters.to ? parseDate(filters.to) : null,
            ]}
            onChange={(range) =>
              url.set({
                from: range?.[0]?.format('YYYY-MM-DD') ?? null,
                to: range?.[1]?.format('YYYY-MM-DD') ?? null,
              })
            }
          />
        </div>
      )}
      <Card variant="borderless" styles={{ body: { padding: 0 } }}>
        <TransactionsTable
          items={transactions.data?.items}
          loading={transactions.isFetching}
          pagination={url.pagination(transactions.data?.total)}
          emptyText={
            tab === 'expected'
              ? 'Ожидаемых платежей нет'
              : filtered
                ? 'Ничего не нашли'
                : 'Платежей пока нет'
          }
        />
      </Card>
    </>
  )

  return (
    <>
      <PageHeader
        title="Платежи"
        subtitle="Платежи по подпискам: ожидаемые появляются в дату следующего платежа"
        extra={
          <>
            <Button icon={<FileExcelOutlined />} loading={exporting} onClick={runExport}>
              Экспорт в Excel
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setAdding(true)}>
              Добавить платёж
            </Button>
          </>
        }
      />
      {transactions.error && (
        <Alert
          type="error"
          showIcon
          title={errorText(transactions.error)}
          style={{ marginBottom: 24 }}
        />
      )}
      <Tabs
        activeKey={tab}
        // Switching tabs drops the filters of «Все».
        onChange={(key) =>
          url.set({
            tab: key === 'all' ? 'all' : null,
            subscription_id: null,
            status: null,
            from: null,
            to: null,
          })
        }
        items={[
          {
            key: 'expected',
            label: (
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                Ожидаемые <Badge count={pending.data?.total ?? 0} size="small" />
              </span>
            ),
            children: table,
          },
          { key: 'all', label: 'Все', children: table },
        ]}
      />
      <TransactionFormModal open={adding} onClose={() => setAdding(false)} />
    </>
  )
}
