import { CheckOutlined, CloseOutlined } from '@ant-design/icons'
import { Alert, App, Button, Form, Input, Modal } from 'antd'
import { useState } from 'react'
import { useApproveRelease, useRejectRelease } from '../api/releases'
import type { ReleaseRequestDTO } from '../api/types'
import { errorText, releaseMovedOn } from '../lib/errors'
import { ModalForm } from './ModalForm'

type Mode = 'approve' | 'reject'

// ReleaseDecisionButtons: «Одобрить» (the licence is revoked) and
// «Отклонить» (it stays), with a comment, required only for a rejection. On
// an error the modal stays open, except when the request moved on meanwhile
// (decided, cancelled, closed with its licence): the card then reloads
// without these buttons and the reason is shown as a message.
export function ReleaseDecisionButtons({ release }: { release: ReleaseRequestDTO }) {
  const { message } = App.useApp()
  const [mode, setMode] = useState<Mode | null>(null)
  const [form] = Form.useForm<{ comment?: string }>()
  const approve = useApproveRelease()
  const reject = useRejectRelease()
  const mutation = mode === 'reject' ? reject : approve

  const close = () => {
    approve.reset()
    reject.reset()
    setMode(null)
  }

  const done = (text: string) => () => {
    message.success(text)
    setMode(null)
  }

  const failed = (e: unknown) => {
    if (releaseMovedOn(e)) {
      message.error(errorText(e))
      close()
    }
  }

  const submit = (values: { comment?: string }) => {
    const comment = values.comment?.trim() ?? ''
    if (mode === 'reject') {
      reject.mutate(
        { id: release.id, comment },
        { onSuccess: done('Отказ отклонён: лицензия остаётся'), onError: failed },
      )
      return
    }
    approve.mutate(
      { id: release.id, comment },
      { onSuccess: done('Отказ принят: лицензия отозвана'), onError: failed },
    )
  }

  return (
    <>
      <Button type="primary" icon={<CheckOutlined />} onClick={() => setMode('approve')}>
        Одобрить
      </Button>
      <Button danger icon={<CloseOutlined />} onClick={() => setMode('reject')}>
        Отклонить
      </Button>
      <Modal
        title={mode === 'reject' ? 'Отклонить отказ' : 'Одобрить отказ'}
        open={mode !== null}
        onCancel={close}
        onOk={() => form.submit()}
        okText={mode === 'reject' ? 'Отклонить' : 'Одобрить и отозвать'}
        okButtonProps={{ danger: mode === 'reject' }}
        confirmLoading={mutation.isPending}
        destroyOnHidden
      >
        <p style={{ marginTop: 0, color: '#949BA4' }}>
          {release.requester_name} отказывается от «{release.subscription_name}».{' '}
          {mode === 'reject'
            ? 'Лицензия останется у сотрудника, причину увидит автор заявки.'
            : 'Лицензия будет отозвана сразу, место освободится.'}
        </p>
        {mutation.error && !releaseMovedOn(mutation.error) && (
          <Alert type="error" showIcon title={errorText(mutation.error)} style={{ marginBottom: 16 }} />
        )}
        <ModalForm form={form} layout="vertical" requiredMark={false} onFinish={submit}>
          <Form.Item
            name="comment"
            label={mode === 'reject' ? 'Почему лицензия ещё нужна' : 'Комментарий (необязательно)'}
            rules={
              mode === 'reject'
                ? [{ required: true, whitespace: true, message: 'Напишите, почему отказ отклонён' }]
                : []
            }
          >
            <Input.TextArea rows={3} maxLength={2000} autoFocus />
          </Form.Item>
        </ModalForm>
      </Modal>
    </>
  )
}
