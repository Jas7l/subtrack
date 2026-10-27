import type { Role } from '../api/types'

export const roleLabels: Record<Role, string> = {
  admin: 'Администратор',
  manager: 'Начальник отдела',
  employee: 'Сотрудник',
}
