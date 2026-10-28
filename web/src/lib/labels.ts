import type {
  AssignmentStatus,
  BillingPeriod,
  RequestStatus,
  Role,
  SubscriptionStatus,
  SubscriptionType,
  TransactionStatus,
} from '../api/types'

export const roleLabels: Record<Role, string> = {
  admin: 'Администратор',
  manager: 'Начальник отдела',
  employee: 'Сотрудник',
}

export const subscriptionTypeLabels: Record<SubscriptionType, string> = {
  individual: 'Индивидуальная',
  group: 'Групповая',
  enterprise: 'Корпоративная (безлимит)',
}

export const subscriptionStatusLabels: Record<SubscriptionStatus, string> = {
  active: 'Активна',
  expiring_soon: 'Скоро истекает',
  expired: 'Истекла',
  cancelled: 'Отменена',
}

export const billingPeriodLabels: Record<BillingPeriod, string> = {
  month: 'Месяц',
  quarter: 'Квартал',
  year: 'Год',
}

export const assignmentStatusLabels: Record<AssignmentStatus, string> = {
  active: 'Выдана',
  revoked: 'Отозвана',
}

export const transactionStatusLabels: Record<TransactionStatus, string> = {
  pending: 'Ожидает оплаты',
  paid: 'Оплачен',
  failed: 'Ошибка оплаты',
}

export const requestStatusLabels: Record<RequestStatus, string> = {
  pending_manager: 'У начальника',
  rejected_by_manager: 'Отклонена начальником',
  pending_admin: 'У администратора',
  rejected_by_admin: 'Отклонена администратором',
  approved: 'Одобрена',
  cancelled: 'Отменена',
}

export const isPending = (status: RequestStatus) =>
  status === 'pending_manager' || status === 'pending_admin'
