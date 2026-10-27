import { Button, Result } from 'antd'
import { useNavigate } from 'react-router'

export function NotFoundPage() {
  const navigate = useNavigate()
  return (
    <Result
      status="404"
      title="Страница не найдена"
      subTitle="Такого адреса нет. Возможно, ссылка устарела."
      extra={
        <Button type="primary" onClick={() => navigate('/')}>
          На главную
        </Button>
      }
    />
  )
}
