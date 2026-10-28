import { Link } from 'react-router'
import { useSubscription } from '../api/subscriptions'
import { ServiceTile } from './ServiceTile'

// SubscriptionLink names a subscription known only by its id (the
// recommendations carry ids). A deleted one stays «#id».
export function SubscriptionLink({ id }: { id: number }) {
  const s = useSubscription(id)
  const name = s.data?.name ?? `#${id}`
  return (
    <Link to={`/subscriptions/${id}`} className="sub-link">
      <ServiceTile name={name} size={24} />
      {name}
    </Link>
  )
}
