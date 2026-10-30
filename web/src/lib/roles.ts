import type { Role } from '../api/types'

// Top menu per role (CLIENT_PLAN.md, section 4). The admin's items are grouped
// so the menu fits one line.
export interface NavLeaf {
  path: string
  label: string
  // Shows the count of requests waiting for the caller's decision, for a
  // subscription and to give a licence back.
  requestsBadge?: boolean
}

export interface NavGroup {
  key: string
  label: string
  children: NavLeaf[]
}

export type NavItem = NavLeaf | NavGroup

export const isGroup = (item: NavItem): item is NavGroup => 'children' in item

export const navByRole: Record<Role, NavItem[]> = {
  admin: [
    { path: '/dashboard', label: 'Дашборд' },
    { path: '/subscriptions', label: 'Подписки' },
    { path: '/requests', label: 'Заявки', requestsBadge: true },
    { path: '/assignments', label: 'Лицензии' },
    { path: '/transactions', label: 'Платежи' },
    {
      key: 'people',
      label: 'Люди',
      children: [
        { path: '/users', label: 'Пользователи' },
        { path: '/departments', label: 'Отделы' },
      ],
    },
    {
      key: 'analytics',
      label: 'Аналитика',
      children: [
        { path: '/recommendations', label: 'Рекомендации' },
        { path: '/forecast', label: 'Прогноз бюджета' },
      ],
    },
    {
      key: 'more',
      label: 'Ещё',
      children: [
        { path: '/audit', label: 'Аудит' },
        { path: '/settings', label: 'Настройки' },
      ],
    },
  ],
  manager: [
    { path: '/dashboard', label: 'Дашборд' },
    { path: '/subscriptions', label: 'Подписки' },
    { path: '/requests', label: 'Заявки', requestsBadge: true },
    { path: '/users', label: 'Сотрудники' },
    { path: '/forecast', label: 'Прогноз' },
    {
      key: 'mine',
      label: 'Моё',
      children: [
        { path: '/my/subscriptions', label: 'Мои подписки' },
        { path: '/my/requests', label: 'Мои заявки' },
      ],
    },
  ],
  employee: [
    { path: '/my/subscriptions', label: 'Мои подписки' },
    { path: '/my/requests', label: 'Мои заявки' },
  ],
}

export function homePath(role: Role): string {
  return role === 'employee' ? '/my/subscriptions' : '/dashboard'
}

// safeNext keeps only an in-app path from ?next=, so the login page cannot be
// used to redirect to another site.
export function safeNext(next: string | null): string | null {
  if (!next || !next.startsWith('/') || next.startsWith('//') || next.startsWith('/login')) {
    return null
  }
  return next
}
