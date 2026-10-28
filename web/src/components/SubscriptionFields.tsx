import { AutoComplete, DatePicker, Form, Input, InputNumber, Select, Space } from 'antd'
import dayjs, { type Dayjs } from 'dayjs'
import { useByCategory } from '../api/analytics'
import { useCurrencies } from '../api/currencies'
import type { BillingPeriod, SubscriptionType } from '../api/types'
import { billingPeriodLabels, subscriptionStatusLabels, subscriptionTypeLabels } from '../lib/labels'
import type { SubscriptionValues } from '../lib/subscriptionForm'
import { DepartmentSelect } from './DepartmentSelect'

const typeOptions = (Object.keys(subscriptionTypeLabels) as SubscriptionType[]).map((t) => ({
  value: t,
  label: subscriptionTypeLabels[t],
}))

const periodOptions = (Object.keys(billingPeriodLabels) as BillingPeriod[]).map((p) => ({
  value: p,
  label: billingPeriodLabels[p],
}))

const typeHints: Record<SubscriptionType, string> = {
  individual: 'Одно место: лицензия для одного человека',
  group: 'Фиксированное число мест',
  enterprise: 'Без ограничения мест',
}

// SubscriptionFields are the fields of a subscription inside the caller's
// Form (creating, editing, approving a request with a new one). The seats
// follow the type: individual — one, hidden; group — required; enterprise —
// none. `seatsUsed` (editing) is the lowest seat count the server accepts.
// An `expired` one (editing) is renewed by moving its end date forward and
// picking «Действует».
export function SubscriptionFields({
  editing = false,
  seatsUsed = 0,
  hideName = false,
  expired = false,
}: {
  editing?: boolean
  seatsUsed?: number
  hideName?: boolean
  expired?: boolean
}) {
  const type = Form.useWatch<SubscriptionType | undefined>('type')
  const currencies = useCurrencies()
  // Categories are free text; the ones in use are offered as hints.
  const categories = useByCategory()
  const categoryOptions = (categories.data ?? [])
    .filter((c) => c.key !== '')
    .map((c) => ({ value: c.key }))

  return (
    <>
      {!hideName && (
        <Form.Item<SubscriptionValues>
          name="name"
          label="Название"
          rules={[{ required: true, whitespace: true, message: 'Укажите название' }]}
        >
          <Input placeholder="Например, Jira" maxLength={200} />
        </Form.Item>
      )}
      <div className="form-row">
        <Form.Item<SubscriptionValues> name="vendor" label="Вендор">
          <Input placeholder="Например, Atlassian" maxLength={200} />
        </Form.Item>
        <Form.Item<SubscriptionValues> name="category" label="Категория">
          <AutoComplete
            options={categoryOptions}
            placeholder="Разработка, дизайн…"
            showSearch={{
              filterOption: (input, option) =>
                (option?.value ?? '').toLowerCase().includes(input.toLowerCase()),
            }}
          />
        </Form.Item>
      </div>
      <div className="form-row">
        <Form.Item<SubscriptionValues>
          name="type"
          label="Тип"
          extra={type && typeHints[type]}
          rules={[{ required: true, message: 'Выберите тип' }]}
        >
          <Select options={typeOptions} />
        </Form.Item>
        {type === 'group' && (
          <Form.Item<SubscriptionValues>
            name="seats_total"
            label="Мест"
            extra={editing && seatsUsed > 0 ? `Занято: ${seatsUsed}` : undefined}
            rules={[
              { required: true, message: 'Укажите число мест' },
              {
                type: 'number',
                min: Math.max(1, seatsUsed),
                message:
                  seatsUsed > 1
                    ? `Не меньше, чем выдано лицензий (${seatsUsed})`
                    : 'Не меньше одного места',
              },
            ]}
          >
            <InputNumber<number> min={1} precision={0} style={{ width: '100%' }} />
          </Form.Item>
        )}
      </div>
      <div className="form-row">
        <Form.Item<SubscriptionValues> label="Стоимость" required>
          <Space.Compact style={{ width: '100%' }}>
            <Form.Item<SubscriptionValues>
              name="cost"
              noStyle
              rules={[{ required: true, message: 'Укажите стоимость' }]}
            >
              <InputNumber<string>
                stringMode
                min="0"
                max="999999999999.99"
                precision={2}
                placeholder="0.00"
                style={{ width: '100%' }}
              />
            </Form.Item>
            <Form.Item<SubscriptionValues> name="currency" noStyle>
              <Select
                style={{ width: 100 }}
                loading={currencies.isPending}
                options={(currencies.data ?? [{ code: 'RUB', name: 'Рубль' }]).map((c) => ({
                  value: c.code,
                  label: c.code,
                  title: c.name,
                }))}
              />
            </Form.Item>
          </Space.Compact>
        </Form.Item>
        <Form.Item<SubscriptionValues>
          name="billing_period"
          label="Период оплаты"
          rules={[{ required: true, message: 'Выберите период' }]}
        >
          <Select options={periodOptions} />
        </Form.Item>
      </div>
      <div className="form-row">
        <Form.Item<SubscriptionValues>
          name="start_date"
          label="Начало"
          rules={[{ required: true, message: 'Укажите дату начала' }]}
        >
          <DatePicker format="DD.MM.YYYY" style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item<SubscriptionValues>
          name="end_date"
          label="Окончание"
          dependencies={['start_date']}
          rules={[
            ({ getFieldValue }) => ({
              validator: (_, end: Dayjs | null | undefined) => {
                const start: Dayjs | undefined = getFieldValue('start_date')
                return end && start && end.isBefore(start, 'day')
                  ? Promise.reject(new Error('Не раньше начала'))
                  : Promise.resolve()
              },
            }),
          ]}
        >
          <DatePicker format="DD.MM.YYYY" placeholder="Бессрочно" style={{ width: '100%' }} />
        </Form.Item>
      </div>
      <div className="form-row">
        <Form.Item<SubscriptionValues> name="next_payment_date" label="Следующий платёж">
          <DatePicker format="DD.MM.YYYY" placeholder="Не задан" style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item<SubscriptionValues> name="department_id" label="Отдел">
          <DepartmentSelect placeholder="Общая (вся компания)" />
        </Form.Item>
      </div>
      {editing && (
        <Form.Item<SubscriptionValues>
          name="status"
          label="Статус"
          extra={
            expired
              ? 'Чтобы продлить: перенесите окончание и выберите «Действует». Прошедшая дата платежа сдвинется вперёд.'
              : '«Отменена» отзывает все лицензии подписки. При возврате в «Действует» прошедшая дата платежа сдвинется вперёд.'
          }
          dependencies={['end_date']}
          rules={[
            ({ getFieldValue }) => ({
              validator: (_, status: SubscriptionValues['status']) => {
                const end: Dayjs | null | undefined = getFieldValue('end_date')
                return expired && status === 'active' && end && end.isBefore(dayjs(), 'day')
                  ? Promise.reject(new Error('Окончание уже прошло: перенесите его на будущее'))
                  : Promise.resolve()
              },
            }),
          ]}
        >
          <Select
            options={[
              ...(expired
                ? [{ value: 'expired', label: subscriptionStatusLabels.expired, disabled: true }]
                : []),
              { value: 'active', label: 'Действует' },
              { value: 'cancelled', label: 'Отменена' },
            ]}
          />
        </Form.Item>
      )}
    </>
  )
}
