import { BellOutlined } from '@ant-design/icons'
import { Badge, Button, Empty, Popover, Spin } from 'antd'
import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useLatestNotifications, useMarkAllRead, useMarkRead } from '../api/notifications'
import type { NotificationDTO } from '../api/types'
import { notificationLink } from '../lib/notifications'
import { colors } from '../theme'
import { NotificationItem } from './NotificationItem'

// NotificationBell: the unread counter and a popup with the latest ten.
export function NotificationBell() {
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const latest = useLatestNotifications()
  const markRead = useMarkRead()
  const markAll = useMarkAllRead()

  const unread = latest.data?.unread ?? 0
  const items = latest.data?.items ?? []

  const openItem = (n: NotificationDTO) => {
    if (!n.is_read) markRead.mutate(n.id)
    const link = notificationLink(n)
    setOpen(false)
    if (link) navigate(link)
  }

  const content = (
    <div className="bell-panel">
      <div className="bell-head">
        <span>Уведомления</span>
        <Button
          type="link"
          size="small"
          disabled={unread === 0}
          loading={markAll.isPending}
          onClick={() => markAll.mutate()}
        >
          Прочитать все
        </Button>
      </div>
      <div className="bell-list">
        {latest.isPending ? (
          <div style={{ padding: 32, textAlign: 'center' }}>
            <Spin />
          </div>
        ) : items.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Уведомлений пока нет" />
        ) : (
          items.map((n) => <NotificationItem key={n.id} item={n} compact onClick={openItem} />)
        )}
      </div>
      <Button
        type="text"
        block
        className="bell-all"
        onClick={() => {
          setOpen(false)
          navigate('/notifications')
        }}
      >
        Все уведомления
      </Button>
    </div>
  )

  return (
    <Popover
      content={content}
      trigger="click"
      placement="bottomRight"
      arrow={false}
      open={open}
      onOpenChange={setOpen}
      styles={{ container: { padding: 0 } }}
    >
      <Badge count={unread} size="small" offset={[-6, 6]}>
        <Button
          aria-label={unread ? `Уведомления: ${unread} новых` : 'Уведомления'}
          icon={<BellOutlined style={{ fontSize: 18 }} />}
          style={{ width: 44, background: colors.bgInput, borderColor: colors.bgInput }}
        />
      </Badge>
    </Popover>
  )
}
