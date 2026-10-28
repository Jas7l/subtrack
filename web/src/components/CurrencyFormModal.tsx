import { Alert, App, Form, Input, InputNumber, Modal } from 'antd'
import { useCreateCurrency, useUpdateCurrency } from '../api/currencies'
import type { CurrencyDTO, UpdateCurrencyBody } from '../api/types'
import { errorText } from '../lib/errors'
import { ModalForm } from './ModalForm'

interface Values {
  code: string
  name: string
  rate_to_rub: string
}

// The server keeps the rate as NUMERIC(18,6) and returns it trimmed ("92.5").
const rate = (v: string) => String(Number(v))

// CurrencyFormModal adds a currency or changes its name and rate. The code is
// set once: subscriptions and payments refer to it.
export function CurrencyFormModal({
  open,
  onClose,
  currency,
}: {
  open: boolean
  onClose: () => void
  currency?: CurrencyDTO
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<Values>()
  const create = useCreateCurrency()
  const update = useUpdateCurrency()
  const mutation = currency ? update : create

  const close = () => {
    create.reset()
    update.reset()
    onClose()
  }

  const submit = (v: Values) => {
    const name = v.name.trim()
    if (!currency) {
      create.mutate(
        {
          code: v.code.trim().toUpperCase(),
          name,
          rate_to_rub: rate(v.rate_to_rub),
        },
        {
          onSuccess: (c) => {
            message.success(`Валюта ${c.code} добавлена`)
            close()
          },
        },
      )
      return
    }
    const body: UpdateCurrencyBody = {}
    if (name !== currency.name) body.name = name
    if (Number(v.rate_to_rub) !== Number(currency.rate_to_rub))
      body.rate_to_rub = rate(v.rate_to_rub)
    if (Object.keys(body).length === 0) {
      close()
      return
    }
    update.mutate(
      { code: currency.code, body },
      {
        onSuccess: () => {
          message.success(`Валюта ${currency.code} сохранена`)
          close()
        },
      },
    )
  }

  return (
    <Modal
      title={currency ? `Валюта ${currency.code}` : 'Новая валюта'}
      open={open}
      onCancel={close}
      onOk={() => form.submit()}
      okText={currency ? 'Сохранить' : 'Добавить'}
      confirmLoading={mutation.isPending}
      destroyOnHidden
    >
      {mutation.error && (
        <Alert
          type="error"
          showIcon
          title={errorText(mutation.error)}
          style={{ marginBottom: 16 }}
        />
      )}
      <ModalForm
        form={form}
        layout="vertical"
        requiredMark={false}
        initialValues={currency}
        onFinish={submit}
      >
        {!currency && (
          <Form.Item
            name="code"
            label="Код"
            extra="Три латинские буквы по ISO 4217, например USD"
            rules={[
              { required: true, message: 'Укажите код' },
              {
                pattern: /^\s*[A-Za-z]{3}\s*$/,
                message: 'Три латинские буквы',
              },
            ]}
          >
            <Input
              placeholder="USD"
              maxLength={3}
              autoFocus
              style={{ textTransform: 'uppercase' }}
            />
          </Form.Item>
        )}
        <Form.Item
          name="name"
          label="Название"
          rules={[{ required: true, whitespace: true, message: 'Укажите название' }]}
        >
          <Input placeholder="Доллар США" maxLength={100} />
        </Form.Item>
        <Form.Item
          name="rate_to_rub"
          label="Курс к рублю"
          extra="Сколько рублей стоит одна единица валюты. Отчёты пересчитываются по текущему курсу."
          rules={[{ required: true, message: 'Укажите курс' }]}
        >
          <InputNumber<string>
            stringMode
            min="0.000001"
            max="999999999999.999999"
            precision={6}
            placeholder="92.5"
            suffix="₽"
            style={{ width: '100%' }}
          />
        </Form.Item>
      </ModalForm>
    </Modal>
  )
}
