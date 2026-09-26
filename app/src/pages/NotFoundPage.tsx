import { Link } from 'react-router-dom'
import { Banner } from '../ui/Banner'

export function NotFoundPage() {
  return (
    <div>
      <h1>Page not found</h1>
      <Banner level="warning">The page you asked for does not exist.</Banner>
      <p><Link to="/tickets">Back to tickets</Link></p>
    </div>
  )
}
