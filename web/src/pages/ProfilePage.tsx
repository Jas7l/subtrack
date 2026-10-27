import { Alert, App, Button, Card, Descriptions, Form, Input } from 'antd'
import { useMutation } from '@tanstack/react-query'
import { authApi } from '../api/auth'
import { useAuth, useMe } from '../auth/AuthContext'
import { PageHeader } from '../components/PageHeader'
import { UserAvatar } from '../components/UserAvatar'
import { errorText } from '../lib/errors'
import { formatDay } from '../lib/format'
import { roleLabels } from '../lib/labels'

const MIN_PASSWORD = 8

interface PasswordValues {
  current: string
  next: string
  confirm: string
}

export function ProfilePage() {
  const me = useMe()
  const { department } = useAuth()
  const { message } = App.useApp()
  const [form] = Form.useForm<PasswordValues>()
  const change = useMutation({
    mutationFn: (v: PasswordValues) => authApi.changePassword(v.current, v.next),
    onSuccess: () => {
      message.success('Пароль изменён')
      form.resetFields()
    },
  })

  return (
    <>
      <PageHeader title="Профиль" />
      <div className="two-columns">
        <Card variant="borderless">
          <div className="profile-head">
            <UserAvatar id={me.id} name={me.full_name} size={72} />
            <div>
              <div className="profile-name">{me.full_name}</div>
              <div className="profile-role">{roleLabels[me.role]}</div>
            </div>
          </div>
          <Descriptions
            column={1}
            colon={false}
            items={[
              { key: 'email', label: 'Email', children: me.email },
              { key: 'department', label: 'Отдел', children: department?.name ?? 'Без отдела' },
              ...(department
                ? [
                    {
                      key: 'head',
                      label: 'Начальник отдела',
                      children: department.head_name ?? 'Не назначен',
                    },
                  ]
                : []),
              { key: 'since', label: 'В системе с', children: formatDay(me.created_at) },
            ]}
          />
        </Card>
        <Card variant="borderless" title="Смена пароля">
          {change.error && (
            <Alert
              type="error"
              showIcon
              title={errorText(change.error)}
              style={{ marginBottom: 20 }}
            />
          )}
          <Form
            form={form}
            layout="vertical"
            requiredMark={false}
            onFinish={(v) => change.mutate(v)}
            onValuesChange={() => change.error && change.reset()}
          >
            <Form.Item
              name="current"
              label="Текущий пароль"
              rules={[{ required: true, message: 'Введите текущий пароль' }]}
            >
              <Input.Password autoComplete="current-password" />
            </Form.Item>
            <Form.Item
              name="next"
              label="Новый пароль"
              rules={[
                { required: true, message: 'Введите новый пароль' },
                { min: MIN_PASSWORD, message: `Не короче ${MIN_PASSWORD} символов` },
              ]}
            >
              <Input.Password autoComplete="new-password" />
            </Form.Item>
            <Form.Item
              name="confirm"
              label="Повторите новый пароль"
              dependencies={['next']}
              rules={[
                { required: true, message: 'Повторите новый пароль' },
                ({ getFieldValue }) => ({
                  validator: (_, value) =>
                    !value || value === getFieldValue('next')
                      ? Promise.resolve()
                      : Promise.reject(new Error('Пароли не совпадают')),
                }),
              ]}
            >
              <Input.Password autoComplete="new-password" />
            </Form.Item>
            <Button type="primary" htmlType="submit" loading={change.isPending}>
              Сменить пароль
            </Button>
          </Form>
        </Card>
      </div>
    </>
  )
}
