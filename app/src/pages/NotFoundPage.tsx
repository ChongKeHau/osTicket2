import { Link } from 'react-router-dom'

export function NotFoundPage() {
  return (
    <div className="panel">
      <h1>Page not found</h1>
      <Link to="/tickets">Back to tickets</Link>
    </div>
  )
}
