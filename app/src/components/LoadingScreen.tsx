export function LoadingScreen({ label = 'Loading…' }: { label?: string }) {
  return <div role="status" aria-live="polite" className="muted" style={{ padding: 24 }}>{label}</div>
}
