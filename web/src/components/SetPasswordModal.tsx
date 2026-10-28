import { Alert, App, Form, Input, Modal } from 'antd'
import type { UserDTO } from '../api/types'
import { useSetPassword } from '../api/users'
import { errorText } from '../lib/errors'

interface Values {
  password: string
  confirm: string
}

// SetPasswordModal: the admin's reset (PUT /users/{id}/password). Issued
// tokens stay valid: the user is not logged out.
export function SetPasswordModal({
  open,
  onClose,
  user,
}: {
  open: boolean
  onClose: () => void
  user: UserDTO
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<Values>()
  const setPassword = useSetPassword()

  const close = () => {
    setPassword.reset()
    onClose()
  }

  const submit = ({ password }: Values) =>
    setPassword.mutate(
      { id: user.id, password },
      {
        onSuccess: () => {
          message.success(`Пароль для ${user.full_name} изменён`)
          close()
        },
      },
    )

  return (
    <Modal
      title="Сбросить пароль"
      open={open}
      onCancel={close}
      onOk={() => form.submit()}
      okText="Сохранить пароль"
      confirmLoading={setPassword.isPending}
      destroyOnHidden
    >
      <p style={{ marginTop: 0, color: '#949BA4' }}>
        Новый пароль для {user.full_name}. Сообщите его пользователю.
      </p>
      {setPassword.error && (
        <Alert
          type="error"
          showIcon
          title={errorText(setPassword.error)}
          style={{ marginBottom: 16 }}
        />
      )}
      <Form form={form} layout="vertical" requiredMark={false} onFinish={submit}>
        <Form.Item
          name="password"
          label="Новый пароль"
          rules={[
            { required: true, message: 'Задайте пароль' },
            { min: 8, message: 'Не короче 8 символов' },
            { max: 72, message: 'Не длиннее 72 символов' },
          ]}
        >
          <Input.Password autoComplete="new-password" autoFocus />
        </Form.Item>
        <Form.Item
          name="confirm"
          label="Повторите пароль"
          dependencies={['password']}
          rules={[
            { required: true, message: 'Повторите пароль' },
            ({ getFieldValue }) => ({
              validator: (_, value: string) =>
                !value || value === getFieldValue('password')
                  ? Promise.resolve()
                  : Promise.reject(new Error('Пароли не совпадают')),
            }),
          ]}
        >
          <Input.Password autoComplete="new-password" />
        </Form.Item>
      </Form>
    </Modal>
  )
}
