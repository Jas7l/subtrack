import { Alert, App, Empty, Form, Input, Modal, Radio, Segmented, Spin } from 'antd'
import { useState } from 'react'
import { useApproveRequest } from '../api/requests'
import { useSubscriptions } from '../api/subscriptions'
import type { ApproveBody, NewSubscriptionBody, RequestDTO } from '../api/types'
import { errorText } from '../lib/errors'
import { toCreateBody, valuesFromRequest, type SubscriptionValues } from '../lib/subscriptionForm'
import { seatsText } from '../lib/subscriptions'
import { SubscriptionStatusBadge } from './StatusBadge'
import { SubscriptionFields } from './SubscriptionFields'
import { SubscriptionSelect } from './SubscriptionSelect'

type Mode = 'existing' | 'new'

type Values = SubscriptionValues & { comment?: string }

// AdminApproveModal is the admin's final approval (CLIENT_PLAN.md, section
// 6): a seat in an existing subscription — the ones matching the request's
// name are offered first, with their free seats — or a new subscription
// prefilled from the request. A full subscription is left selectable: the
// server's 409 «no free seats» is shown and the modal stays open.
export function AdminApproveModal({
  open,
  onClose,
  request: r,
}: {
  open: boolean
  onClose: () => void
  request: RequestDTO
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<Values>()
  const [mode, setMode] = useState<Mode>('existing')
  const [subscriptionId, setSubscriptionId] = useState<number>()
  const [missingChoice, setMissingChoice] = useState(false)
  const approve = useApproveRequest()
  const matches = useSubscriptions({ q: r.name, limit: 10 }, open)
  const items = matches.data?.items ?? []

  const close = () => {
    approve.reset()
    setSubscriptionId(undefined)
    setMissingChoice(false)
    setMode('existing')
    onClose()
  }

  const send = (body: ApproveBody) =>
    approve.mutate(
      { id: r.id, body },
      {
        onSuccess: () => {
          message.success('Заявка одобрена, лицензия выдана')
          close()
        },
      },
    )

  const submit = (v: Values) => {
    const comment = v.comment?.trim()
    const base: ApproveBody = comment ? { comment } : {}
    if (mode === 'existing') {
      if (subscriptionId === undefined) {
        setMissingChoice(true)
        return
      }
      send({ ...base, subscription_id: subscriptionId })
      return
    }
    // department_id is always sent: absent would mean «the request's».
    const subscription: NewSubscriptionBody = {
      ...toCreateBody(v),
      department_id: v.department_id ?? null,
    }
    send({ ...base, subscription })
  }

  const pick = (id: number | undefined) => {
    setSubscriptionId(id)
    setMissingChoice(false)
  }

  return (
    <Modal
      title={`Одобрить заявку «${r.name}»`}
      open={open}
      onCancel={close}
      onOk={() => form.submit()}
      okText="Одобрить и выдать лицензию"
      confirmLoading={approve.isPending}
      width={720}
      destroyOnHidden
    >
      <p style={{ marginTop: 0, color: '#949BA4' }}>
        {r.requester_name}
        {r.department_name && ` · ${r.department_name}`}. Лицензия будет выдана автору заявки.
      </p>
      {r.status === 'pending_manager' && (
        <Alert
          type="warning"
          showIcon
          title="Заявка ещё у начальника отдела"
          description="Решить её за начальника можно, только если у отдела его нет."
          style={{ marginBottom: 16 }}
        />
      )}
      {approve.error && (
        <Alert type="error" showIcon title={errorText(approve.error)} style={{ marginBottom: 16 }} />
      )}
      <Segmented<Mode>
        block
        value={mode}
        onChange={setMode}
        options={[
          { value: 'existing', label: 'Место в существующей подписке' },
          { value: 'new', label: 'Новая подписка' },
        ]}
        style={{ marginBottom: 20 }}
      />
      <Form
        form={form}
        layout="vertical"
        requiredMark={false}
        initialValues={valuesFromRequest(r)}
        onFinish={submit}
      >
        {mode === 'existing' ? (
          <>
            <div className="approve-matches">
              <div className="cell-sub" style={{ marginBottom: 8 }}>
                Похожие по названию «{r.name}»
              </div>
              {matches.isPending ? (
                <Spin />
              ) : items.length === 0 ? (
                <Empty
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                  description="Похожих подписок нет — выберите другую ниже или создайте новую"
                />
              ) : (
                <Radio.Group
                  value={subscriptionId}
                  onChange={(e) => pick(e.target.value as number)}
                  style={{ display: 'flex', flexDirection: 'column', gap: 8 }}
                  options={items.map((s) => ({
                    value: s.id,
                    label: (
                      <span className="approve-match">
                        <span className="cell-title">{s.name}</span>
                        <span className="cell-sub">
                          {s.department_name ?? 'Общая'} · места {seatsText(s)}
                          {s.seats_free === 0 && ' · мест нет'}
                        </span>
                        <SubscriptionStatusBadge status={s.status} />
                      </span>
                    ),
                  }))}
                />
              )}
            </div>
            <Form.Item
              label="Другая подписка"
              validateStatus={missingChoice ? 'error' : undefined}
              help={missingChoice ? 'Выберите подписку' : undefined}
            >
              <SubscriptionSelect value={subscriptionId} onChange={pick} allowClear />
            </Form.Item>
          </>
        ) : (
          <SubscriptionFields />
        )}
        <Form.Item name="comment" label="Комментарий (необязательно)">
          <Input.TextArea rows={2} maxLength={2000} />
        </Form.Item>
      </Form>
    </Modal>
  )
}
