import { App, Button, Popconfirm } from 'antd'
import { useCancelRequest } from '../api/requests'
import type { RequestDTO } from '../api/types'
import { errorText } from '../lib/errors'

// CancelRequestButton lets the requester withdraw a request that still waits.
export function CancelRequestButton({
  request,
  size,
}: {
  request: RequestDTO
  size?: 'small' | 'middle'
}) {
  const { message } = App.useApp()
  const cancel = useCancelRequest()
  return (
    <Popconfirm
      title="Отменить заявку?"
      okText="Отменить заявку"
      cancelText="Нет"
      okButtonProps={{ danger: true }}
      onConfirm={() =>
        cancel.mutateAsync(request.id).then(
          () => message.success('Заявка отменена'),
          (err: unknown) => message.error(errorText(err)),
        )
      }
    >
      <Button size={size} loading={cancel.isPending}>
        Отменить
      </Button>
    </Popconfirm>
  )
}
