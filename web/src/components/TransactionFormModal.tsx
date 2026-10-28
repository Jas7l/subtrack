import {
  Alert,
  App,
  AutoComplete,
  DatePicker,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
} from 'antd'
import dayjs, { type Dayjs } from 'dayjs'
import { useCurrencies } from '../api/currencies'
import { useCreateTransaction } from '../api/transactions'
import type { CreateTransactionBody, SubscriptionDTO } from '../api/types'
import { errorText } from '../lib/errors'
import { paymentMethods } from '../lib/transactions'
import { ModalForm } from './ModalForm'
import { SubscriptionSelect } from './SubscriptionSelect'

interface Values {
  subscription_id: number
  amount: string
  currency: string
  payment_date: Dayjs
  payment_method?: string
  invoice_number?: string
}

// TransactionFormModal: a payment entered by hand (POST /transactions) —
// a one-off or a past one; it is stored as paid. On the subscription's card
// the subscription is fixed; otherwise it is picked, and its cost and
// currency fill the amount. Cancelled and expired subscriptions are allowed
// (history). One payment per subscription and date: a second one is a 409.
export function TransactionFormModal({
  open,
  onClose,
  subscription,
}: {
  open: boolean
  onClose: () => void
  subscription?: SubscriptionDTO
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<Values>()
  const currencies = useCurrencies()
  const create = useCreateTransaction()

  const close = () => {
    create.reset()
    onClose()
  }

  const submit = (v: Values) => {
    const body: CreateTransactionBody = {
      subscription_id: subscription?.id ?? v.subscription_id,
      amount: Number(v.amount).toFixed(2),
      currency: v.currency,
      payment_date: v.payment_date.format('YYYY-MM-DD'),
    }
    const method = v.payment_method?.trim()
    const invoice = v.invoice_number?.trim()
    if (method) body.payment_method = method
    if (invoice) body.invoice_number = invoice
    create.mutate(body, {
      onSuccess: () => {
        message.success('Платёж добавлен')
        close()
      },
    })
  }

  // Picking a subscription suggests its cost.
  const picked = (s: SubscriptionDTO | undefined) => {
    if (s) form.setFieldsValue({ amount: s.cost, currency: s.currency })
  }

  return (
    <Modal
      title={subscription ? `Платёж по «${subscription.name}»` : 'Новый платёж'}
      open={open}
      onCancel={close}
      onOk={() => form.submit()}
      okText="Добавить"
      confirmLoading={create.isPending}
      destroyOnHidden
    >
      <p className="cell-sub" style={{ marginTop: 0 }}>
        Платёж, внесённый вручную, сразу считается оплаченным.
      </p>
      {create.error && (
        <Alert type="error" showIcon title={errorText(create.error)} style={{ marginBottom: 16 }} />
      )}
      <ModalForm
        form={form}
        layout="vertical"
        requiredMark={false}
        initialValues={{
          amount: subscription?.cost,
          currency: subscription?.currency ?? 'RUB',
          payment_date: dayjs(),
        }}
        onFinish={submit}
      >
        {!subscription && (
          <Form.Item
            name="subscription_id"
            label="Подписка"
            rules={[{ required: true, message: 'Выберите подписку' }]}
          >
            <SubscriptionSelect onChange={(_, s) => picked(s)} />
          </Form.Item>
        )}
        <div className="form-row">
          <Form.Item<Values> label="Сумма" required>
            <Space.Compact style={{ width: '100%' }}>
              <Form.Item<Values>
                name="amount"
                noStyle
                rules={[{ required: true, message: 'Укажите сумму' }]}
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
              <Form.Item<Values> name="currency" noStyle>
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
          <Form.Item<Values>
            name="payment_date"
            label="Дата платежа"
            rules={[{ required: true, message: 'Укажите дату' }]}
          >
            <DatePicker format="DD.MM.YYYY" allowClear={false} style={{ width: '100%' }} />
          </Form.Item>
        </div>
        <Form.Item<Values> name="payment_method" label="Способ оплаты">
          <AutoComplete options={paymentMethods} placeholder="Не указан" maxLength={200} />
        </Form.Item>
        <Form.Item<Values> name="invoice_number" label="Номер счёта">
          <Input placeholder="Например, INV-2026-090" maxLength={200} />
        </Form.Item>
      </ModalForm>
    </Modal>
  )
}
