import { ApiError } from '../api/client'

export function ErrorBanner({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const message = error instanceof ApiError ? error.message : error instanceof Error ? error.message : 'Something went wrong'
  return (
    <div role="alert" className="panel" style={{ borderColor: 'var(--danger)', color: 'var(--danger)' }}>
      <span>{message}</span>
      {onRetry && <button type="button" onClick={onRetry} style={{ marginLeft: 12 }}>Retry</button>}
    </div>
  )
}
