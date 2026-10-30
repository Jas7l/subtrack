import { Link } from 'react-router'
import { StatusBadge } from './StatusBadge'

// ReleasePendingLink marks a licence whose holder asked to give it back and
// opens the request (a row click underneath does not fire).
export function ReleasePendingLink({ id }: { id: number }) {
  return (
    <Link to={`/release-requests/${id}`} onClick={(e) => e.stopPropagation()}>
      <StatusBadge tone="warning">Просит отказ</StatusBadge>
    </Link>
  )
}
