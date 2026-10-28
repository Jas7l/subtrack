import type { MeDTO, RequestDTO, SubscriptionDTO } from '../api/types'

// The rules below repeat the server's, so the buttons match what it allows
// (it checks anyway). A manager without a department manages nothing.

// inboxStatus is the request status waiting for this user's decision: a
// department head decides pending_manager, an admin pending_admin.
export function inboxStatus(me: MeDTO, isHead: boolean): 'pending_manager' | 'pending_admin' | null {
  if (me.role === 'admin') return 'pending_admin'
  if (me.role === 'manager' && isHead) return 'pending_manager'
  return null
}

// canDecide: the head of the request's department, while it waits for them.
// The admin's decisions are К4.
export function canDecide(me: MeDTO, isHead: boolean, r: RequestDTO): boolean {
  return (
    isHead &&
    r.status === 'pending_manager' &&
    me.department_id !== null &&
    r.department_id === me.department_id
  )
}

// canManageSubscription: who hands out and revokes licences. Any manager of
// the owning department (not only its head); company-wide subscriptions are
// the admin's (К4).
export function canManageSubscription(me: MeDTO, sub: SubscriptionDTO): boolean {
  return (
    me.role === 'manager' && me.department_id !== null && sub.department_id === me.department_id
  )
}
