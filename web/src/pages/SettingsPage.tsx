import { DeleteOutlined, EditOutlined, LockOutlined, PlusOutlined } from '@ant-design/icons'
import {
  Alert,
  App,
  Button,
  Card,
  Form,
  InputNumber,
  Popconfirm,
  Space,
  Table,
  Tabs,
  Tooltip,
  type TableProps,
} from 'antd'
import { useState } from 'react'
import { useCurrencies, useDeleteCurrency } from '../api/currencies'
import { useNotificationSettings, useUpdateNotificationSettings } from '../api/settings'
import type { CurrencyDTO, NotificationSettingsBody } from '../api/types'
import { CurrencyFormModal } from '../components/CurrencyFormModal'
import { PageHeader } from '../components/PageHeader'
import { errorText } from '../lib/errors'
import { formatMoment } from '../lib/format'
import { useUrlState } from '../lib/urlState'

// SettingsPage (admin): when the scheduler warns («Уведомления») and the
// currency rates reports convert with («Валюты»).
export function SettingsPage() {
  const url = useUrlState()
  const tab = url.get('tab') === 'currencies' ? 'currencies' : 'notifications'

  return (
    <>
      <PageHeader title="Настройки" subtitle="Напоминания и курсы валют" />
      <Tabs
        activeKey={tab}
        onChange={(key) => url.set({ tab: key === 'currencies' ? key : null })}
        items={[
          {
            key: 'notifications',
            label: 'Уведомления',
            children: <NotificationSettings />,
          },
          { key: 'currencies', label: 'Валюты', children: <Currencies /> },
        ]}
      />
    </>
  )
}

// The server accepts 0..365 days.
const MAX_DAYS = 365

function NotificationSettings() {
  const { message } = App.useApp()
  const [form] = Form.useForm<NotificationSettingsBody>()
  const settings = useNotificationSettings()
  const update = useUpdateNotificationSettings()

  if (!settings.data) {
    return settings.error ? <Alert type="error" showIcon title={errorText(settings.error)} /> : null
  }

  const submit = (v: NotificationSettingsBody) =>
    update.mutate(v, {
      onSuccess: () => message.success('Настройки сохранены'),
    })

  const days = (label: string, extra: string, name: keyof NotificationSettingsBody) => (
    <Form.Item
      name={name}
      label={label}
      extra={extra}
      rules={[{ required: true, message: 'Укажите число дней' }]}
    >
      <InputNumber<number>
        min={0}
        max={MAX_DAYS}
        precision={0}
        suffix="дн."
        style={{ width: 160 }}
      />
    </Form.Item>
  )

  return (
    <Card variant="borderless" style={{ maxWidth: 640 }}>
      {update.error && (
        <Alert type="error" showIcon title={errorText(update.error)} style={{ marginBottom: 16 }} />
      )}
      <Form
        form={form}
        layout="vertical"
        requiredMark={false}
        initialValues={settings.data}
        onFinish={submit}
      >
        {days(
          'Окончание подписки',
          'За сколько дней до окончания подписка становится «Скоро истекает»: держатели лицензий и администраторы получают уведомление.',
          'expiring_days',
        )}
        {days(
          'Платёж',
          'За сколько дней до следующего платежа администраторам приходит напоминание. 0 — в день платежа.',
          'payment_days',
        )}
        <Space size="middle" align="center">
          <Button type="primary" htmlType="submit" loading={update.isPending}>
            Сохранить
          </Button>
          <span className="cell-sub">Изменено {formatMoment(settings.data.updated_at)}</span>
        </Space>
      </Form>
    </Card>
  )
}

function Currencies() {
  const { message } = App.useApp()
  const currencies = useCurrencies()
  const remove = useDeleteCurrency()
  // undefined — closed, null — a new currency.
  const [editing, setEditing] = useState<CurrencyDTO | null>()

  const columns: TableProps<CurrencyDTO>['columns'] = [
    {
      title: 'Код',
      key: 'code',
      render: (_, c) => <span className="cell-title">{c.code}</span>,
    },
    { title: 'Название', key: 'name', dataIndex: 'name' },
    {
      title: 'Курс к рублю',
      key: 'rate',
      align: 'right',
      render: (_, c) => (
        <span className="cell-money">
          {Number(c.rate_to_rub).toLocaleString('ru-RU', {
            maximumFractionDigits: 6,
          })}{' '}
          ₽
        </span>
      ),
    },
    {
      title: 'Обновлён',
      key: 'updated',
      render: (_, c) => formatMoment(c.updated_at),
    },
    {
      key: 'actions',
      align: 'right',
      render: (_, c) =>
        c.code === 'RUB' ? (
          <Tooltip title="Основная валюта отчётов: курс всегда 1, удалить нельзя">
            <LockOutlined className="cell-sub" />
          </Tooltip>
        ) : (
          <Space>
            <Button size="small" icon={<EditOutlined />} onClick={() => setEditing(c)}>
              Изменить
            </Button>
            <Popconfirm
              title={`Удалить валюту ${c.code}?`}
              description="Удалить можно только валюту, которой нет в подписках, заявках и платежах."
              okText="Удалить"
              cancelText="Оставить"
              okButtonProps={{ danger: true }}
              onConfirm={() =>
                remove.mutateAsync(c.code).then(
                  () => message.success(`Валюта ${c.code} удалена`),
                  (err: unknown) => message.error(errorText(err)),
                )
              }
            >
              <Button size="small" danger icon={<DeleteOutlined />} aria-label="Удалить" />
            </Popconfirm>
          </Space>
        ),
    },
  ]

  return (
    <>
      <div className="filters" style={{ justifyContent: 'space-between', alignItems: 'center' }}>
        <span className="cell-sub">
          Курсы задаются вручную. Отчёты пересчитывают суммы в рубли по текущему курсу.
        </span>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setEditing(null)}>
          Добавить валюту
        </Button>
      </div>
      {currencies.error && (
        <Alert
          type="error"
          showIcon
          title={errorText(currencies.error)}
          style={{ marginBottom: 24 }}
        />
      )}
      <Card variant="borderless" styles={{ body: { padding: 0 } }}>
        <Table<CurrencyDTO>
          rowKey="code"
          columns={columns}
          dataSource={currencies.data}
          loading={currencies.isFetching}
          pagination={false}
        />
      </Card>
      <CurrencyFormModal
        open={editing !== undefined}
        onClose={() => setEditing(undefined)}
        currency={editing ?? undefined}
      />
    </>
  )
}
