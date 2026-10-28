import { Alert, Result, Spin } from 'antd'
import { Link } from 'react-router'
import { ApiError } from '../api/client'
import { errorText } from '../lib/errors'

// QueryState stands for a card page while its record is not there: a
// spinner, «not found» (a 404, or `missing` for a bad id in the address) or
// the error.
export function QueryState({
  error,
  missing = false,
  notFound,
}: {
  error: unknown
  missing?: boolean
  notFound: string
}) {
  if (missing || (error instanceof ApiError && error.status === 404)) {
    return (
      <Result
        status="404"
        title={notFound}
        subTitle="Запись удалена или вам не видна."
        extra={<Link to="/">На главную</Link>}
      />
    )
  }
  if (error) return <Alert type="error" showIcon title={errorText(error)} />
  return (
    <div className="page-spin">
      <Spin size="large" />
    </div>
  )
}

// NoDepartment: a manager outside any department has no department numbers
// (the server answers 403) and no staff.
export function NoDepartment({ title }: { title: string }) {
  return (
    <Result
      status="info"
      title={title}
      subTitle="Вы не привязаны к отделу. Попросите администратора добавить вас в отдел."
    />
  )
}
