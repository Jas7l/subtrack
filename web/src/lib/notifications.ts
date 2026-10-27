import type { NotificationDTO } from '../api/types'

// notificationLink is the page a notification opens; null for old
// notifications without a link (they stay on the list). A transaction has no
// page of its own yet and opens nothing.
export function notificationLink(n: NotificationDTO): string | null {
  if (n.entity_id === null) return null
  switch (n.entity_type) {
    case 'request':
      return `/requests/${n.entity_id}`
    case 'subscription':
      return `/subscriptions/${n.entity_id}`
    default:
      return null
  }
}
