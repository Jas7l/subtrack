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
import { useRelease } from '../api/releases'
import type { ReleaseRequestDTO } from '../api/types'
import { useMe } from '../auth/AuthContext'
import { CancelReleaseButton } from '../components/CancelReleaseButton'
import { PageHeader } from '../components/PageHeader'
import { QueryState } from '../components/QueryState'
import { ReleaseDecisionButtons } from '../components/ReleaseDecisionButtons'
import { ServiceTile } from '../components/ServiceTile'
import { ReleaseStatusBadge } from '../components/StatusBadge'
import { formatDay, formatMoment } from '../lib/format'
import { canDecideRelease, managesDepartment } from '../lib/permissions'
import { colors } from '../theme'

type StepItem = NonNullable<StepsProps['items']>[number]

// who decides r: whoever manages the subscription.
function reviewers(r: ReleaseRequestDTO): string {
  return r.department_name
    ? `Начальник отдела «${r.department_name}» или администратор`
    : 'Администратор (подписка общая)'
}

// releaseSteps: submitted → decision → result. One decision: a manager of
// the subscription's department or the admin, whoever is first.
function releaseSteps(r: ReleaseRequestDTO): StepItem[] {
  const decision: StepItem = { title: 'Решение' }
  const result: StepItem = { title: 'Итог' }
  switch (r.status) {
    case 'pending':
      decision.status = 'process'
      decision.content = reviewers(r)
      result.status = 'wait'
      break
    case 'approved':
      decision.status = 'finish'
      decision.content = r.decided_by_name ?? 'Одобрена'
      result.status = 'finish'
      result.content = 'Лицензия отозвана'
      break
    case 'rejected':
      decision.status = 'error'
      decision.content = r.decided_by_name ?? 'Отклонена'
      result.status = 'error'
      result.content = 'Лицензия осталась'
      break
    case 'cancelled':
      decision.status = 'wait'
      decision.content = '—'
      result.status = 'finish'
      result.title = 'Отменена'
      result.icon = <StopFilled style={{ color: colors.textMuted }} />
      break
  }
  return [{ title: 'Подана', status: 'finish', content: formatDay(r.created_at) }, decision, result]
}

interface Event {
  icon: ReactNode
  title: ReactNode
  when: string
  comment?: string
}

// releaseHistory lists who did what, oldest first.
function releaseHistory(r: ReleaseRequestDTO): Event[] {
  const events: Event[] = [
    {
      icon: <FileAddFilled style={{ color: colors.accent }} />,
      title: <>Подал(а) {r.requester_name}</>,
      when: r.created_at,
      comment: r.reason,
    },
  ]
  if (r.decided_at && r.status === 'approved') {
    events.push({
      icon: <CheckCircleFilled style={{ color: colors.success }} />,
      title: <>Лицензию отозвал(а) {r.decided_by_name ?? ''}</>,
      when: r.decided_at,
      comment: r.comment,
    })
  }
  if (r.decided_at && r.status === 'rejected') {
    events.push({
      icon: <CloseCircleFilled style={{ color: colors.error }} />,
      title: <>Отклонил(а) {r.decided_by_name ?? ''}</>,
      when: r.decided_at,
      comment: r.comment,
    })
  }
  if (r.status === 'cancelled') {
    events.push({
      icon: <StopFilled style={{ color: colors.textMuted }} />,
      title: 'Заявка отменена',
      when: r.updated_at,
    })
  }
  return events
}

// ReleaseRequestPage: a request to give a licence back. Whoever manages the
// subscription sees «Одобрить» / «Отклонить» while it waits, the requester
// «Отменить».
export function ReleaseRequestPage() {
  const id = Number(useParams().id)
  const me = useMe()
  const query = useRelease(id)

  if (!query.data) {
    return (
      <QueryState
        error={query.error}
        missing={!Number.isInteger(id)}
        notFound="Заявка на отказ не найдена"
      />
    )
  }

  const r = query.data
  const steps = releaseSteps(r)
  const current = Math.max(
    0,
    steps.findIndex((s) => s.status === 'process' || s.status === 'error'),
  )
  const actions = canDecideRelease(me, r) ? (
    <ReleaseDecisionButtons release={r} />
  ) : (
    me.id === r.requester_id && r.status === 'pending' && <CancelReleaseButton id={r.id} />
  )
  // The subscription opens for whoever manages it, and for the holder while
  // the licence is still theirs.
  const opensSubscription = managesDepartment(me, r.department_id) || r.status !== 'approved'

  return (
    <>
      <PageHeader
        title={
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 16 }}>
            <ServiceTile name={r.subscription_name} size={52} />
            <span>
              Отказ от «{r.subscription_name}»
              <span className="page-title-id"> №{r.id}</span>
            </span>
          </span>
        }
        subtitle={
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 12 }}>
            <ReleaseStatusBadge status={r.status} />
            {r.requester_name}
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
              {
                key: 'subscription',
                label: 'Подписка',
                children: opensSubscription ? (
                  <Link to={`/subscriptions/${r.subscription_id}`}>
                    <LinkOutlined /> {r.subscription_name}
                  </Link>
                ) : (
                  r.subscription_name
                ),
              },
              { key: 'requester', label: 'Сотрудник', children: r.requester_name },
              {
                key: 'department',
                label: 'Отдел подписки',
                children: r.department_name ?? 'Общая на компанию',
              },
              { key: 'reviewers', label: 'Кто решает', children: reviewers(r) },
              { key: 'reason', label: 'Причина', children: r.reason || '—' },
            ]}
          />
        </Card>
        <Card variant="borderless" title="История">
          <Timeline
            items={releaseHistory(r).map((e, i) => ({
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
