import { CheckOutlined, CloseOutlined } from '@ant-design/icons'
import { Alert, App, Button, Form, Input, Modal } from 'antd'
import { useState } from 'react'
import { useApproveRequest, useRejectRequest } from '../api/requests'
import type { ApproveBody, RequestDTO } from '../api/types'
import { useMe } from '../auth/AuthContext'
import { errorText } from '../lib/errors'
import { AdminApproveModal } from './AdminApproveModal'

type Mode = 'approve' | 'reject'

// DecisionButtons: «Одобрить» and «Отклонить» with a comment, required only
// for a rejection. The admin's approval also picks the subscription
// (AdminApproveModal). On an error the modal stays open.
export function DecisionButtons({ request }: { request: RequestDTO }) {
  const me = useMe()
  const admin = me.role === 'admin'
  const { message } = App.useApp()
  const [mode, setMode] = useState<Mode | null>(null)
  const [form] = Form.useForm<{ comment?: string }>()
  const approve = useApproveRequest()
  const reject = useRejectRequest()
  const mutation = mode === 'reject' ? reject : approve

  const close = () => {
    approve.reset()
    reject.reset()
    setMode(null)
  }

  const done = (text: string) => () => {
    message.success(text)
    form.resetFields()
    setMode(null)
  }

  const submit = (values: { comment?: string }) => {
    const comment = values.comment?.trim() ?? ''
    if (mode === 'reject') {
      reject.mutate({ id: request.id, comment }, { onSuccess: done('Заявка отклонена') })
      return
    }
    // The server rejects an empty body: an approval without a comment is {}.
    const body: ApproveBody = {}
    if (comment) body.comment = comment
    approve.mutate({ id: request.id, body }, { onSuccess: done('Заявка одобрена') })
  }

  return (
    <>
      <Button type="primary" icon={<CheckOutlined />} onClick={() => setMode('approve')}>
        {admin ? 'Одобрить…' : 'Одобрить'}
      </Button>
      <Button danger icon={<CloseOutlined />} onClick={() => setMode('reject')}>
        Отклонить
      </Button>
      {admin && (
        <AdminApproveModal open={mode === 'approve'} onClose={close} request={request} />
      )}
      <Modal
        title={mode === 'reject' ? 'Отклонить заявку' : 'Одобрить заявку'}
        open={mode === 'reject' || (mode === 'approve' && !admin)}
        onCancel={close}
        onOk={() => form.submit()}
        okText={mode === 'reject' ? 'Отклонить' : 'Одобрить'}
        okButtonProps={{ danger: mode === 'reject' }}
        confirmLoading={mutation.isPending}
        destroyOnHidden
      >
        <p style={{ marginTop: 0, color: '#949BA4' }}>
          «{request.name}» от {request.requester_name}.{' '}
          {mode === 'reject'
            ? 'Автор увидит причину отказа.'
            : 'Заявка уйдёт администратору на финальное решение.'}
        </p>
        {mutation.error && (
          <Alert type="error" showIcon title={errorText(mutation.error)} style={{ marginBottom: 16 }} />
        )}
        <Form form={form} layout="vertical" requiredMark={false} onFinish={submit}>
          <Form.Item
            name="comment"
            label={mode === 'reject' ? 'Причина отказа' : 'Комментарий (необязательно)'}
            rules={
              mode === 'reject'
                ? [{ required: true, whitespace: true, message: 'Напишите, почему отказ' }]
                : []
            }
          >
            <Input.TextArea rows={3} maxLength={2000} autoFocus />
          </Form.Item>
        </Form>
      </Modal>
    </>
  )
}
