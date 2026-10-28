import type {
  AuditEntityType,
  AuditEntryDTO,
  BillingPeriod,
  RequestStatus,
  Role,
  SubscriptionStatus,
  SubscriptionType,
  TransactionStatus,
} from '../api/types'
import type { Tone } from '../theme'
import { formatDate, formatMoney } from './format'
import {
  billingPeriodLabels,
  requestStatusLabels,
  roleLabels,
  subscriptionStatusLabels,
  subscriptionTypeLabels,
  transactionStatusLabels,
} from './labels'

export const entityLabels: Record<AuditEntityType, string> = {
  user: 'Пользователь',
  department: 'Отдел',
  subscription: 'Подписка',
  assignment: 'Лицензия',
  request: 'Заявка',
  transaction: 'Платёж',
  settings: 'Настройки',
  currency: 'Валюта',
}

// Every action the server records (internal/*/service.go), by entity.
export const actionLabels: Record<string, string> = {
  'user.create': 'Пользователь создан',
  'user.update': 'Пользователь изменён',
  'user.password_change': 'Пароль сменён',
  'user.password_reset': 'Пароль сброшен',
  'department.create': 'Отдел создан',
  'department.update': 'Отдел изменён',
  'department.delete': 'Отдел удалён',
  'subscription.create': 'Подписка создана',
  'subscription.update': 'Подписка изменена',
  'subscription.delete': 'Подписка удалена',
  'assignment.create': 'Лицензия выдана',
  'assignment.revoke': 'Лицензия отозвана',
  'request.create': 'Заявка создана',
  'request.approve': 'Заявка одобрена',
  'request.reject': 'Заявка отклонена',
  'request.cancel': 'Заявка отменена',
  'transaction.create': 'Платёж создан',
  'transaction.pay': 'Платёж оплачен',
  'transaction.fail': 'Ошибка оплаты',
  'currency.create': 'Валюта добавлена',
  'currency.update': 'Валюта изменена',
  'currency.delete': 'Валюта удалена',
  'settings.update': 'Настройки изменены',
}

// An unknown action is shown as the server wrote it.
export const actionText = (action: string) => actionLabels[action] ?? action

const verbTones: Record<string, Tone> = {
  create: 'success',
  approve: 'success',
  pay: 'success',
  update: 'info',
  password_change: 'info',
  password_reset: 'info',
  reject: 'warning',
  cancel: 'warning',
  delete: 'error',
  revoke: 'error',
  fail: 'error',
}

export const actionTone = (action: string): Tone => verbTones[action.split('.')[1]] ?? 'muted'

const num = (v: unknown) => (typeof v === 'number' ? v : undefined)
const str = (v: unknown) => (typeof v === 'string' ? v : undefined)

// entityLink: where the record's object lives. A payment and a licence have
// no page of their own: they open their subscription when the details name
// it. null — nowhere (a deleted subscription still opens as «not found»).
export function entityLink(e: AuditEntryDTO): string | null {
  const id = e.entity_id
  const subId = num(e.details.subscription_id)
  switch (e.entity_type) {
    case 'subscription':
      return id === null ? null : `/subscriptions/${id}`
    case 'request':
      return id === null ? null : `/requests/${id}`
    case 'user':
      return id === null ? null : `/users/${id}`
    case 'department':
      return '/departments'
    case 'assignment':
      return subId ? `/subscriptions/${subId}` : '/assignments'
    case 'transaction':
      return subId ? `/subscriptions/${subId}` : '/transactions'
    case 'currency':
      return '/settings?tab=currencies'
    case 'settings':
      return '/settings'
    default:
      return null
  }
}

// entityText: «Подписка «Jira»», «Лицензия: Jira → Иванов», «Валюта USD».
export function entityText(e: AuditEntryDTO): string {
  const d = e.details
  const label = entityLabels[e.entity_type] ?? e.entity_type
  const name = str(d.name) ?? str(d.full_name)
  switch (e.entity_type) {
    case 'assignment':
      return d.subscription && d.user ? `${label}: ${str(d.subscription)} → ${str(d.user)}` : `${label} #${e.entity_id}`
    case 'currency':
      return `${label} ${str(d.code) ?? ''}`.trim()
    case 'settings':
      return 'Настройки уведомлений'
    default:
      if (name) return `${label} «${name}»`
      return e.entity_id === null ? label : `${label} #${e.entity_id}`
  }
}

// One line of the details: a Russian name and a readable value.
export interface DetailRow {
  key: string
  label: string
  value: string
  // A page the value points at (a subscription, a user).
  to?: string
}

const fieldLabels: Record<string, string> = {
  name: 'Название',
  full_name: 'ФИО',
  email: 'Email',
  role: 'Роль',
  department_id: 'Отдел',
  head_id: 'Начальник',
  is_active: 'Доступ',
  status: 'Статус',
  type: 'Тип',
  vendor: 'Вендор',
  category: 'Категория',
  cost: 'Стоимость',
  amount: 'Сумма',
  currency: 'Валюта',
  billing_period: 'Период оплаты',
  start_date: 'Начало',
  end_date: 'Окончание',
  next_payment_date: 'Следующий платёж',
  payment_date: 'Дата платежа',
  seats_total: 'Мест',
  licences_revoked: 'Отозвано лицензий',
  subscription_id: 'Подписка',
  user_id: 'Сотрудник',
  stage: 'Решение',
  comment: 'Комментарий',
  payment_method: 'Способ оплаты',
  invoice_number: 'Номер счёта',
  code: 'Код',
  rate_to_rub: 'Курс к рублю',
  bootstrap: 'Создан',
  expiring_days: 'Предупреждать об окончании, дней',
  payment_days: 'Предупреждать о платеже, дней',
}

// Keys shown inside another row (a name next to its id, the old rate or
// status before the new one) or not worth a line.
const hidden = new Set(['subscription', 'user', 'old_rate_to_rub', 'from', 'by'])

const dateFields = new Set(['start_date', 'end_date', 'next_payment_date', 'payment_date'])

function statusText(entity: AuditEntityType, status: string): string {
  switch (entity) {
    case 'subscription':
      return subscriptionStatusLabels[status as SubscriptionStatus] ?? status
    case 'request':
      return requestStatusLabels[status as RequestStatus] ?? status
    case 'transaction':
      return transactionStatusLabels[status as TransactionStatus] ?? status
    default:
      return status
  }
}

// detailRows turns the details into lines, the known keys first in the
// order above, unknown ones after them as they are. `departments` names the
// department ids.
export function detailRows(e: AuditEntryDTO, departments: Map<number, string>): DetailRow[] {
  const d = e.details
  const currency = str(d.currency) ?? 'RUB'
  const known = Object.keys(fieldLabels).filter((k) => k in d)
  const unknown = Object.keys(d).filter((k) => !(k in fieldLabels) && !hidden.has(k))
  return [...known, ...unknown].map((key) => {
    const v = d[key]
    const row = (value: string, to?: string): DetailRow => ({
      key,
      label: fieldLabels[key] ?? key,
      value,
      to,
    })
    if (v === null || v === undefined || v === '') {
      return row(key === 'department_id' ? 'без отдела' : key === 'head_id' ? 'без начальника' : '—')
    }
    switch (key) {
      case 'role':
        return row(roleLabels[v as Role] ?? String(v))
      case 'is_active':
        return row(v ? 'разблокирован' : 'заблокирован')
      case 'status':
        return row(
          d.from
            ? `${statusText(e.entity_type, String(d.from))} → ${statusText(e.entity_type, String(v))}`
            : statusText(e.entity_type, String(v)),
        )
      case 'bootstrap':
        return row('при первом запуске сервера')
      case 'type':
        return row(subscriptionTypeLabels[v as SubscriptionType] ?? String(v))
      case 'billing_period':
        return row(billingPeriodLabels[v as BillingPeriod] ?? String(v))
      case 'cost':
      case 'amount':
        return row(formatMoney(String(v), currency))
      case 'department_id':
        return row(departments.get(Number(v)) ?? `#${String(v)}`)
      case 'head_id':
        return row(`пользователь #${String(v)}`, `/users/${String(v)}`)
      case 'subscription_id':
        return row(str(d.subscription) ?? `#${String(v)}`, `/subscriptions/${String(v)}`)
      case 'user_id':
        return row(str(d.user) ?? `#${String(v)}`, `/users/${String(v)}`)
      case 'stage':
        return row(v === 'manager' ? 'начальника отдела' : v === 'admin' ? 'администратора' : String(v))
      case 'rate_to_rub':
        return row(d.old_rate_to_rub ? `${String(d.old_rate_to_rub)} → ${String(v)} ₽` : `${String(v)} ₽`)
      default:
        if (dateFields.has(key)) return row(formatDate(String(v)))
        return row(typeof v === 'object' ? JSON.stringify(v) : String(v))
    }
  })
}

// detailSummary: the details in one short line for the table.
export function detailSummary(rows: DetailRow[]): string {
  return rows.map((r) => `${r.label}: ${r.value}`).join(' · ')
}
