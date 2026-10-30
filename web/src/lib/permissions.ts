import type { MeDTO, ReleaseRequestDTO, RequestDTO, SubscriptionDTO } from '../api/types'

// The rules below repeat the server's, so the buttons match what it allows
// (it checks anyway). A manager without a department manages nothing.

// inboxStatus is the request status waiting for this user's decision: a
// department head decides pending_manager, an admin pending_admin.
export function inboxStatus(me: MeDTO, isHead: boolean): 'pending_manager' | 'pending_admin' | null {
  if (me.role === 'admin') return 'pending_admin'
  if (me.role === 'manager' && isHead) return 'pending_manager'
  return null
}

// canDecide: the head of the request's department while it waits for them;
// the admin on pending_admin, and on pending_manager too — that one is theirs
// only when the department has no head, otherwise the server answers 409
// «waits for the head», shown as is. Nobody decides their own request (made
// before they became the head or the admin): the server answers 403.
export function canDecide(me: MeDTO, isHead: boolean, r: RequestDTO): boolean {
  if (r.requester_id === me.id) return false
  if (me.role === 'admin') return r.status === 'pending_admin' || r.status === 'pending_manager'
  return (
    isHead &&
    r.status === 'pending_manager' &&
    me.department_id !== null &&
    r.department_id === me.department_id
  )
}

// managesDepartment: whoever manages the subscriptions of departmentId — hands
// out and revokes their licences, decides the requests to give them back.
// The admin — any; a manager — their department's (not only its head);
// company-wide subscriptions (null) are the admin's.
export function managesDepartment(me: MeDTO, departmentId: number | null): boolean {
  if (me.role === 'admin') return true
  return me.role === 'manager' && me.department_id !== null && departmentId === me.department_id
}

export function canManageSubscription(me: MeDTO, sub: SubscriptionDTO): boolean {
  return managesDepartment(me, sub.department_id)
}

// hasReleaseInbox: who gets release requests to decide — the admin (those
// nobody else can decide) and a manager with a department (its own).
export function hasReleaseInbox(me: MeDTO): boolean {
  return me.role === 'admin' || (me.role === 'manager' && me.department_id !== null)
}

// canDecideRelease: whoever manages the subscription, while the request
// waits. The admin may decide any, not only those in their inbox. The
// requester too, if they came to manage the subscription meanwhile: they
// may revoke the licence anyway.
export function canDecideRelease(me: MeDTO, r: ReleaseRequestDTO): boolean {
  return r.status === 'pending' && managesDepartment(me, r.department_id)
}
