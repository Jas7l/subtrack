import { BarChartOutlined, BellOutlined, CreditCardOutlined } from '@ant-design/icons'
import { Alert, Button, Form, Input } from 'antd'
import { useState } from 'react'
import { Navigate, useNavigate, useSearchParams } from 'react-router'
import { ApiError } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import { Logo } from '../components/Logo'
import { errorText } from '../lib/errors'
import { homePath, safeNext } from '../lib/roles'

interface Credentials {
  email: string
  password: string
}

// Demo accounts from `make seed`, for quick sign-in: shown in development
// (Vite) and in a demo build (`make demo`, VITE_DEMO=1), never in a regular
// one. The admin's password comes from .env, so that button fills just the
// email.
const showDemo = import.meta.env.DEV || import.meta.env.VITE_DEMO === '1'
const demoAccounts = [
  { label: 'Админ', email: 'admin@company.ru', password: '' },
  { label: 'Начальник', email: 'ivanov@company.ru', password: 'password123' },
  { label: 'Сотрудник', email: 'popov@company.ru', password: 'password123' },
]

const features = [
  {
    icon: <CreditCardOutlined />,
    title: 'Лицензии без таблиц.',
    text: 'Кто чем пользуется и сколько мест свободно.',
  },
  {
    icon: <BellOutlined />,
    title: 'Напоминания вовремя.',
    text: 'Об окончании подписок и ближайших платежах.',
  },
  {
    icon: <BarChartOutlined />,
    title: 'Расходы под контролем.',
    text: 'Аналитика, прогноз бюджета и советы по экономии.',
  },
]

function loginError(err: unknown): string {
  if (err instanceof ApiError && err.status === 401) return 'Неверный email или пароль'
  if (err instanceof ApiError && err.status === 403) return 'Учётная запись заблокирована'
  return errorText(err)
}

export function LoginPage() {
  const { me, login } = useAuth()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const [form] = Form.useForm<Credentials>()
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const next = safeNext(params.get('next'))
  if (me) return <Navigate to={next ?? homePath(me.role)} replace />

  const submit = async ({ email, password }: Credentials) => {
    setError(null)
    setSubmitting(true)
    try {
      const user = await login(email.trim(), password)
      navigate(next ?? homePath(user.role), { replace: true })
    } catch (err) {
      setError(loginError(err))
      setSubmitting(false)
    }
  }

  const fillDemo = (email: string, password: string) => {
    form.setFieldsValue({ email, password })
    if (password) form.submit()
  }

  return (
    <div className="login">
      <section className="login-about">
        <Logo size={44} />
        <div style={{ display: 'flex', flexDirection: 'column', gap: 28 }}>
          <h1>Все подписки компании — в одном месте</h1>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>
            {features.map((f) => (
              <div className="login-feature" key={f.title}>
                <span className="icon-soft">{f.icon}</span>
                <p>
                  <b>{f.title}</b> {f.text}
                </p>
              </div>
            ))}
          </div>
        </div>
        <p style={{ margin: 0, fontSize: 13, color: '#949BA4' }}>Учёт подписок на ПО для предприятия</p>
      </section>

      <section className="login-form-side">
        <div className="login-form">
          <h2>С возвращением!</h2>
          <p style={{ margin: '0 0 28px', fontSize: 16, color: '#949BA4' }}>
            Войдите корпоративной почтой
          </p>
          <Form form={form} layout="vertical" size="large" requiredMark={false} onFinish={submit}>
            <Form.Item
              name="email"
              label="Email"
              rules={[
                { required: true, message: 'Введите email' },
                { type: 'email', message: 'Неверный формат email' },
              ]}
            >
              <Input autoComplete="username" autoFocus />
            </Form.Item>
            <Form.Item
              name="password"
              label="Пароль"
              rules={[{ required: true, message: 'Введите пароль' }]}
            >
              <Input.Password autoComplete="current-password" />
            </Form.Item>
            {error && <Alert type="error" title={error} showIcon style={{ marginBottom: 20 }} />}
            <Button type="primary" htmlType="submit" block loading={submitting}>
              Войти
            </Button>
          </Form>

          {showDemo && (
            <div style={{ marginTop: 32 }}>
              <p style={{ margin: '0 0 10px', fontSize: 13, color: '#949BA4' }}>
                Демо-аккаунты
              </p>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                {demoAccounts.map((a) => (
                  <Button key={a.email} onClick={() => fillDemo(a.email, a.password)}>
                    {a.label}
                  </Button>
                ))}
              </div>
            </div>
          )}
        </div>
      </section>
    </div>
  )
}
