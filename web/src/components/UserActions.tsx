import { EditOutlined, KeyOutlined, LockOutlined, MoreOutlined, UnlockOutlined } from '@ant-design/icons'
import { App, Button, Dropdown, type MenuProps } from 'antd'
import { useState } from 'react'
import type { UserDTO } from '../api/types'
import { useUpdateUser } from '../api/users'
import { useMe } from '../auth/AuthContext'
import { errorText } from '../lib/errors'
import { SetPasswordModal } from './SetPasswordModal'
import { UserFormModal } from './UserFormModal'

type Action = 'edit' | 'password' | 'block'

// UserActions are the admin's actions on a user: edit, reset the password,
// block or unblock. The same actions sit in a «⋯» menu of a table row
// (`menu`) and as buttons in the user's card. Blocking warns first: it
// revokes every licence and leaves a department without its head.
export function UserActions({ user, variant }: { user: UserDTO; variant: 'menu' | 'buttons' }) {
  const me = useMe()
  const { message, modal } = App.useApp()
  const [open, setOpen] = useState<'edit' | 'password' | null>(null)
  const update = useUpdateUser()
  const self = user.id === me.id

  const setActive = (active: boolean) =>
    update.mutateAsync({ id: user.id, body: { is_active: active } }).then(
      () => message.success(active ? `${user.full_name} разблокирован(а)` : `${user.full_name} заблокирован(а)`),
      (err: unknown) => message.error(errorText(err)),
    )

  const toggleBlock = () => {
    if (!user.is_active) {
      void setActive(true)
      return
    }
    modal.confirm({
      title: `Заблокировать ${user.full_name}?`,
      content: (
        <>
          <p>Вход в систему закроется сразу, все лицензии пользователя будут отозваны.</p>
          {user.role === 'manager' && (
            <p style={{ marginBottom: 0 }}>
              Если пользователь руководит отделом, отдел останется без начальника.
            </p>
          )}
        </>
      ),
      okText: 'Заблокировать',
      okButtonProps: { danger: true },
      cancelText: 'Отмена',
      // The modal waits for the server and closes either way; an error is a
      // message.
      onOk: () => setActive(false),
    })
  }

  const run = (action: Action) => {
    if (action === 'block') toggleBlock()
    else setOpen(action)
  }

  const blockLabel = user.is_active ? 'Заблокировать' : 'Разблокировать'
  const blockIcon = user.is_active ? <LockOutlined /> : <UnlockOutlined />

  const modals = (
    <>
      <UserFormModal open={open === 'edit'} onClose={() => setOpen(null)} user={user} />
      <SetPasswordModal open={open === 'password'} onClose={() => setOpen(null)} user={user} />
    </>
  )

  if (variant === 'buttons') {
    return (
      <>
        <Button icon={<EditOutlined />} onClick={() => run('edit')}>
          Редактировать
        </Button>
        <Button icon={<KeyOutlined />} onClick={() => run('password')}>
          Сбросить пароль
        </Button>
        {!self && (
          <Button
            danger={user.is_active}
            icon={blockIcon}
            loading={update.isPending}
            onClick={() => run('block')}
          >
            {blockLabel}
          </Button>
        )}
        {modals}
      </>
    )
  }

  const items: MenuProps['items'] = [
    { key: 'edit', icon: <EditOutlined />, label: 'Редактировать' },
    { key: 'password', icon: <KeyOutlined />, label: 'Сбросить пароль' },
    { type: 'divider' },
    {
      key: 'block',
      icon: blockIcon,
      label: self ? 'Себя заблокировать нельзя' : blockLabel,
      danger: user.is_active && !self,
      disabled: self,
    },
  ]

  return (
    // A row click opens the card: the menu must not pass clicks through.
    <span onClick={(e) => e.stopPropagation()}>
      <Dropdown
        trigger={['click']}
        menu={{ items, onClick: ({ key, domEvent }) => {
          domEvent.stopPropagation()
          run(key as Action)
        } }}
      >
        <Button type="text" icon={<MoreOutlined />} loading={update.isPending} aria-label="Действия" />
      </Dropdown>
      {modals}
    </span>
  )
}
