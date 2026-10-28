import { Alert, App, AutoComplete, Descriptions, Form, Input, Modal } from 'antd'
import { usePayTransaction } from '../api/transactions'
import type { PayTransactionBody, TransactionDTO } from '../api/types'
import { errorText } from '../lib/errors'
import { formatDate, formatMoney } from '../lib/format'
import { paymentMethods } from '../lib/transactions'

interface Values {
  payment_method?: string
  invoice_number?: string
}

// PayTransactionModal: POST /transactions/{id}/pay. The method and the invoice
// number are optional; filled-in ones are stored with the payment.
export function PayTransactionModal({
  transaction: t,
  onClose,
}: {
  transaction: TransactionDTO | undefined
  onClose: () => void
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<Values>()
  const pay = usePayTransaction()

  const close = () => {
    pay.reset()
    onClose()
  }

  const submit = (v: Values) => {
    if (!t) return
    const body: PayTransactionBody = {}
    const method = v.payment_method?.trim()
    const invoice = v.invoice_number?.trim()
    if (method) body.payment_method = method
    if (invoice) body.invoice_number = invoice
    pay.mutate(
      { id: t.id, body },
      {
        onSuccess: () => {
          message.success('Платёж отмечен оплаченным')
          close()
        },
      },
    )
  }

  return (
    <Modal
      title="Отметить оплату"
      open={t !== undefined}
      onCancel={close}
      onOk={() => form.submit()}
      okText="Оплачено"
      confirmLoading={pay.isPending}
      destroyOnHidden
    >
      {t && (
        <Descriptions
          column={1}
          colon={false}
          size="small"
          style={{ marginBottom: 16 }}
          items={[
            { key: 'sub', label: 'Подписка', children: t.subscription_name },
            {
              key: 'amount',
              label: 'Сумма',
              children: formatMoney(t.amount, t.currency),
            },
            {
              key: 'date',
              label: 'Дата платежа',
              children: formatDate(t.payment_date),
            },
          ]}
        />
      )}
      {pay.error && (
        <Alert type="error" showIcon title={errorText(pay.error)} style={{ marginBottom: 16 }} />
      )}
      <Form
        form={form}
        layout="vertical"
        requiredMark={false}
        initialValues={{
          payment_method: t?.payment_method || undefined,
          invoice_number: t?.invoice_number || undefined,
        }}
        onFinish={submit}
      >
        <Form.Item name="payment_method" label="Способ оплаты">
          <AutoComplete options={paymentMethods} placeholder="Не указан" maxLength={200} />
        </Form.Item>
        <Form.Item name="invoice_number" label="Номер счёта">
          <Input placeholder="Например, INV-2026-114" maxLength={200} />
        </Form.Item>
      </Form>
    </Modal>
  )
}
