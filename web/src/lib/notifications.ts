import { useQueryClient } from '@tanstack/react-query'
import { App } from 'antd'
import { useMarkRead } from '../api/notifications'
import type { NotificationDTO, NotificationEntity } from '../api/types'
import { errorText } from './errors'

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

// What a notification about each kind of thing may have changed: a decided
// request also hands out a licence.
const staleAfter: Partial<Record<NotificationEntity, string[]>> = {
  request: ['requests', 'subscriptions', 'assignments'],
  subscription: ['subscriptions', 'assignments'],
  transaction: ['transactions'],
}

// useOpenNotification is a click on a notification: it marks it read and
// refreshes what it is about — the page it opens would otherwise show the
// cached state from before the event (a request still pending, an inbox
// without the new one). It returns the page to open, or null.
export function useOpenNotification() {
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const markRead = useMarkRead()
  return (n: NotificationDTO): string | null => {
    if (!n.is_read) markRead.mutate(n.id, { onError: (err) => message.error(errorText(err)) })
    for (const key of (n.entity_type && staleAfter[n.entity_type]) || []) {
      void queryClient.invalidateQueries({ queryKey: [key] })
    }
    return notificationLink(n)
  }
}
