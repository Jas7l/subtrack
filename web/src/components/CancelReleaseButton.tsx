import { App, Button, Popconfirm } from 'antd'
import { useCancelRelease } from '../api/releases'
import { errorText } from '../lib/errors'

// CancelReleaseButton lets the requester withdraw a release request that
// still waits: the licence stays theirs.
export function CancelReleaseButton({
  id,
  size,
  label = 'Отменить',
}: {
  id: number
  size?: 'small' | 'middle'
  label?: string
}) {
  const { message } = App.useApp()
  const cancel = useCancelRelease()
  return (
    <Popconfirm
      title="Отменить заявку на отказ?"
      description="Лицензия останется за вами."
      okText="Отменить заявку"
      cancelText="Нет"
      okButtonProps={{ danger: true }}
      onConfirm={() =>
        cancel.mutateAsync(id).then(
          () => message.success('Заявка на отказ отменена'),
          (err: unknown) => message.error(errorText(err)),
        )
      }
    >
      <Button size={size} loading={cancel.isPending}>
        {label}
      </Button>
    </Popconfirm>
  )
}
