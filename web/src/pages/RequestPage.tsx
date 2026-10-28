import {
  CheckCircleFilled,
  CloseCircleFilled,
  FileAddFilled,
  LinkOutlined,
  StopFilled,
} from '@ant-design/icons'
import { Card, Descriptions, Steps, Timeline, type StepsProps } from 'antd'
import type { ReactNode } from 'react'
import { Link, useParams } from 'react-router'
import { useRequest } from '../api/requests'
import type { RequestDTO } from '../api/types'
import { useAuth, useMe } from '../auth/AuthContext'
import { CancelRequestButton } from '../components/CancelRequestButton'
import { DecisionButtons } from '../components/DecisionButtons'
import { PageHeader } from '../components/PageHeader'
import { ServiceTile } from '../components/ServiceTile'
import { QueryState } from '../components/QueryState'
import { RequestStatusBadge } from '../components/StatusBadge'
import { formatDay, formatMoment, formatMoney } from '../lib/format'
import { isPending } from '../lib/labels'
import { canDecide } from '../lib/permissions'
import { colors } from '../theme'

type StepItem = NonNullable<StepsProps['items']>[number]

// requestSteps draws the request state machine (CLIENT_PLAN.md, section 6):
// submitted → head → admin → result. A head's request, or one from a
// department without a head, goes straight to the admin: the head step is
// then skipped.
function requestSteps(r: RequestDTO): StepItem[] {
  const s = r.status
  const headDecided = r.manager_decided_at !== null
  const skippedHead = !headDecided && s !== 'pending_manager' && s !== 'cancelled'

  const head: StepItem = { title: 'Начальник отдела' }
  if (s === 'pending_manager') {
    head.status = 'process'
    head.content = 'Ждёт решения'
  } else if (s === 'rejected_by_manager') {
    head.status = 'error'
    head.content = r.manager_name ?? 'Отклонена'
  } else if (headDecided) {
    head.status = 'finish'
    head.content = r.manager_name ?? 'Согласовано'
  } else {
    head.status = 'wait'
    head.content = skippedHead ? 'Не требуется' : '—'
  }

  const admin: StepItem = { title: 'Администратор' }
  if (s === 'pending_admin') {
    admin.status = 'process'
    admin.content = 'Ждёт решения'
  } else if (s === 'rejected_by_admin') {
    admin.status = 'error'
    admin.content = r.admin_name ?? 'Отклонена'
  } else if (s === 'approved') {
    admin.status = 'finish'
    admin.content = r.admin_name ?? 'Одобрена'
  } else {
    admin.status = 'wait'
  }

  const result: StepItem = { title: 'Итог' }
  if (s === 'approved') {
    result.status = 'finish'
    result.content = 'Лицензия выдана'
  } else if (s === 'rejected_by_manager' || s === 'rejected_by_admin') {
    result.status = 'error'
    result.content = 'Отклонена'
  } else if (s === 'cancelled') {
    result.status = 'finish'
    result.title = 'Отменена'
    result.content = 'Автором заявки'
    result.icon = <StopFilled style={{ color: colors.textMuted }} />
  } else {
    result.status = 'wait'
  }

  return [
    { title: 'Подана', status: 'finish', content: formatDay(r.created_at) },
    head,
    admin,
    result,
  ]
}

interface Event {
  icon: ReactNode
  title: ReactNode
  when: string
  comment?: string
}

// requestHistory lists who did what, oldest first.
function requestHistory(r: RequestDTO): Event[] {
  const events: Event[] = [
    {
      icon: <FileAddFilled style={{ color: colors.accent }} />,
      title: <>Подал(а) {r.requester_name}</>,
      when: r.created_at,
    },
  ]
  if (r.manager_decided_at) {
    const rejected = r.status === 'rejected_by_manager'
    events.push({
      icon: rejected ? (
        <CloseCircleFilled style={{ color: colors.error }} />
      ) : (
        <CheckCircleFilled style={{ color: colors.success }} />
      ),
      title: (
        <>
          {rejected ? 'Отклонил(а)' : 'Согласовал(а)'} начальник отдела {r.manager_name ?? ''}
        </>
      ),
      when: r.manager_decided_at,
      comment: r.manager_comment,
    })
  }
  if (r.admin_decided_at) {
    const rejected = r.status === 'rejected_by_admin'
    events.push({
      icon: rejected ? (
        <CloseCircleFilled style={{ color: colors.error }} />
      ) : (
        <CheckCircleFilled style={{ color: colors.success }} />
      ),
      title: (
        <>
          {rejected ? 'Отклонил(а)' : 'Одобрил(а)'} администратор {r.admin_name ?? ''}
        </>
      ),
      when: r.admin_decided_at,
      comment: r.admin_comment,
    })
  }
  if (r.status === 'cancelled') {
    events.push({
      icon: <StopFilled style={{ color: colors.textMuted }} />,
      title: 'Автор отменил заявку',
      when: r.updated_at,
    })
  }
  return events
}

export function RequestPage() {
  const id = Number(useParams().id)
  const me = useMe()
  const { isHead } = useAuth()
  const query = useRequest(id)

  if (!query.data) {
    return <QueryState error={query.error} missing={!Number.isInteger(id)} notFound="Заявка не найдена" />
  }

  const r = query.data
  const steps = requestSteps(r)
  const current = Math.max(
    0,
    steps.findIndex((s) => s.status === 'process' || s.status === 'error'),
  )

  // Buttons by role and status (CLIENT_PLAN.md, section 6); the admin's
  // decisions are К4.
  const actions = canDecide(me, isHead, r) ? (
    <DecisionButtons request={r} />
  ) : (
    me.id === r.requester_id && isPending(r.status) && <CancelRequestButton request={r} />
  )

  return (
    <>
      <PageHeader
        title={
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 16 }}>
            <ServiceTile name={r.name} size={52} />
            <span>
              {r.name}
              <span className="page-title-id"> №{r.id}</span>
            </span>
          </span>
        }
        subtitle={
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 12 }}>
            <RequestStatusBadge status={r.status} />
            {r.requester_name}
            {r.department_name && ` · ${r.department_name}`}
          </span>
        }
        extra={actions}
      />

      <Card variant="borderless" style={{ marginBottom: 24 }}>
        <Steps items={steps} current={current} />
      </Card>

      <div className="two-columns">
        <Card variant="borderless" title="Заявка">
          <Descriptions
            column={1}
            colon={false}
            items={[
              { key: 'name', label: 'Сервис', children: r.name },
              { key: 'vendor', label: 'Вендор', children: r.vendor || '—' },
              {
                key: 'cost',
                label: 'Примерная стоимость',
                children: formatMoney(r.estimated_cost, r.currency),
              },
              { key: 'department', label: 'Отдел', children: r.department_name ?? 'Без отдела' },
              { key: 'requester', label: 'Автор', children: r.requester_name },
              { key: 'justification', label: 'Зачем нужна', children: r.justification },
              ...(r.subscription_id !== null
                ? [
                    {
                      key: 'subscription',
                      label: 'Подписка',
                      children: (
                        <Link to={`/subscriptions/${r.subscription_id}`}>
                          <LinkOutlined /> Открыть подписку
                        </Link>
                      ),
                    },
                  ]
                : []),
            ]}
          />
        </Card>
        <Card variant="borderless" title="История">
          <Timeline
            items={requestHistory(r).map((e, i) => ({
              key: i,
              icon: e.icon,
              content: (
                <div className="history-item">
                  <div className="history-title">{e.title}</div>
                  <div className="history-when">{formatMoment(e.when)}</div>
                  {e.comment && <div className="history-comment">{e.comment}</div>}
                </div>
              ),
            }))}
          />
        </Card>
      </div>
    </>
  )
}
