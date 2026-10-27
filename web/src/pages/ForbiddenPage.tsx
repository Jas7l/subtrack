import { Button, Result } from 'antd'
import { useNavigate } from 'react-router'

export function ForbiddenPage() {
  const navigate = useNavigate()
  return (
    <Result
      status="403"
      title="Нет доступа"
      subTitle="Этот раздел недоступен для вашей роли."
      extra={
        <Button type="primary" onClick={() => navigate('/')}>
          На главную
        </Button>
      }
    />
  )
}
