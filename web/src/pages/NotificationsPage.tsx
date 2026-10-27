import { CheckOutlined } from '@ant-design/icons'
import { Alert, App, Button, Card, Empty, Pagination, Segmented, Spin } from 'antd'
import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useMarkAllRead, useMarkRead, useNotifications } from '../api/notifications'
import type { NotificationDTO } from '../api/types'
import { NotificationItem } from '../components/NotificationItem'
import { PageHeader } from '../components/PageHeader'
import { errorText } from '../lib/errors'
import { notificationLink } from '../lib/notifications'

const PAGE_SIZE = 20

export function NotificationsPage() {
  const navigate = useNavigate()
  const { message } = App.useApp()
  const [unreadOnly, setUnreadOnly] = useState(false)
  const [page, setPage] = useState(1)

  const list = useNotifications({
    unread: unreadOnly || undefined,
    limit: PAGE_SIZE,
    offset: (page - 1) * PAGE_SIZE,
  })
  const markRead = useMarkRead()
  const markAll = useMarkAllRead()

  const unread = list.data?.unread ?? 0
  const items = list.data?.items ?? []

  // A click marks the notification read and opens what it is about; old
  // notifications without a link just get marked.
  const open = (n: NotificationDTO) => {
    if (!n.is_read) markRead.mutate(n.id)
    const link = notificationLink(n)
    if (link) navigate(link)
  }

  return (
    <>
      <PageHeader
        title="Уведомления"
        subtitle={unread > 0 ? `Непрочитанных: ${unread}` : 'Всё прочитано'}
        extra={
          <>
            <Segmented
              value={unreadOnly ? 'unread' : 'all'}
              options={[
                { value: 'all', label: 'Все' },
                { value: 'unread', label: 'Непрочитанные' },
              ]}
              onChange={(v) => {
                setUnreadOnly(v === 'unread')
                setPage(1)
              }}
            />
            <Button
              icon={<CheckOutlined />}
              disabled={unread === 0}
              loading={markAll.isPending}
              onClick={() =>
                markAll.mutate(undefined, { onError: (err) => message.error(errorText(err)) })
              }
            >
              Прочитать все
            </Button>
          </>
        }
      />
      {list.error && (
        <Alert type="error" showIcon title={errorText(list.error)} style={{ marginBottom: 24 }} />
      )}
      <Card variant="borderless" styles={{ body: { padding: 8 } }}>
        {list.isPending ? (
          <div className="page-spin">
            <Spin size="large" />
          </div>
        ) : items.length === 0 ? (
          <Empty
            style={{ padding: 32 }}
            description={unreadOnly ? 'Непрочитанных уведомлений нет' : 'Уведомлений пока нет'}
          />
        ) : (
          items.map((n) => <NotificationItem key={n.id} item={n} onClick={open} />)
        )}
      </Card>
      <Pagination
        style={{ marginTop: 20, justifyContent: 'center' }}
        current={page}
        pageSize={PAGE_SIZE}
        total={list.data?.total ?? 0}
        hideOnSinglePage
        showSizeChanger={false}
        onChange={setPage}
      />
    </>
  )
}
