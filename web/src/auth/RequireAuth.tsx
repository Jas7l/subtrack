import { Button, Result, Spin } from 'antd'
import type { ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router'
import { errorText } from '../lib/errors'
import { useAuth } from './AuthContext'

// RequireAuth lets through only a signed-in user whose profile is loaded;
// otherwise it sends to /login and remembers where the user was going —
// except after «Выйти»: the next one to sign in starts from their home page.
export function RequireAuth({ children }: { children: ReactNode }) {
  const { token, me, loading, error, signedOut } = useAuth()
  const location = useLocation()

  if (!token) {
    if (signedOut) return <Navigate to="/login" replace />
    const next = encodeURIComponent(location.pathname + location.search)
    return <Navigate to={`/login?next=${next}`} replace />
  }
  if (loading) {
    return (
      <div className="full-center">
        <Spin size="large" />
      </div>
    )
  }
  if (!me) {
    return (
      <div className="full-center">
        <Result
          status="error"
          title="Не удалось загрузить профиль"
          subTitle={errorText(error)}
          extra={
            <Button type="primary" onClick={() => window.location.reload()}>
              Повторить
            </Button>
          }
        />
      </div>
    )
  }
  return children
}
