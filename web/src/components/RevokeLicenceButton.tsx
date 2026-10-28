import { App, Button, Popconfirm } from 'antd'
import { useRevokeAssignment } from '../api/assignments'
import type { AssignmentDTO } from '../api/types'
import { errorText } from '../lib/errors'

// RevokeLicenceButton takes a licence back from its holder (a head of the
// owning department, the admin).
export function RevokeLicenceButton({ assignment }: { assignment: AssignmentDTO }) {
  const { message } = App.useApp()
  const revoke = useRevokeAssignment()
  return (
    <Popconfirm
      title="Отозвать лицензию?"
      description={`${assignment.user_name} потеряет доступ к «${assignment.subscription_name}».`}
      okText="Отозвать"
      cancelText="Оставить"
      okButtonProps={{ danger: true }}
      onConfirm={() =>
        revoke.mutateAsync(assignment.id).then(
          () => message.success(`Лицензия отозвана: ${assignment.user_name}`),
          (err: unknown) => message.error(errorText(err)),
        )
      }
    >
      <Button size="small" danger loading={revoke.isPending}>
        Отозвать
      </Button>
    </Popconfirm>
  )
}
