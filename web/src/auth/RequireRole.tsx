import type { ReactNode } from 'react'
import type { Role } from '../api/types'
import { ForbiddenPage } from '../pages/ForbiddenPage'
import { useMe } from './AuthContext'

// RequireRole shows «Нет доступа» on a route of another role. The server
// checks rights anyway; this only keeps the user out of dead ends.
export function RequireRole({ roles, children }: { roles: readonly Role[]; children: ReactNode }) {
  const me = useMe()
  return roles.includes(me.role) ? children : <ForbiddenPage />
}
