import { Button, Result } from 'antd'
import { useNavigate } from 'react-router'

// ErrorPage stands in for a page that crashed, or whose code failed to load
// (a tab left open over a new build): reloading usually helps. fullScreen is
// for errors outside the pages, when the menu is gone too.
export function ErrorPage({ fullScreen = false }: { fullScreen?: boolean }) {
  const navigate = useNavigate()
  const result = (
    <Result
      status="500"
      title="Что-то пошло не так"
      subTitle="Страница не открылась из-за ошибки. Обновите её — обычно это помогает."
      extra={[
        <Button key="reload" type="primary" onClick={() => window.location.reload()}>
          Обновить страницу
        </Button>,
        <Button key="home" onClick={() => navigate('/')}>
          На главную
        </Button>,
      ]}
    />
  )
  return fullScreen ? <div className="full-center">{result}</div> : result
}
