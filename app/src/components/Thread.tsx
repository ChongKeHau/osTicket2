import { useInfiniteQuery } from '@tanstack/react-query'
import { getThread } from '../api/tickets'
import { ErrorBanner } from './ErrorBanner'
import { LoadingScreen } from './LoadingScreen'
import { ThreadEntry } from './ThreadEntry'

export function Thread({ ticketId }: { ticketId: number }) {
  const q = useInfiniteQuery({
    queryKey: ['thread', ticketId],
    queryFn: ({ pageParam }) => getThread(ticketId, pageParam),
    initialPageParam: 0,
    getNextPageParam: (last) => last.next_after ?? undefined,
  })
  if (q.isLoading) return <LoadingScreen label="Loading thread…" />
  if (q.error) return <ErrorBanner error={q.error} onRetry={() => void q.refetch()} />
  const entries = q.data?.pages.flatMap((p) => p.items) ?? []
  return (
    <section aria-label="Thread">
      <h2>Thread</h2>
      {entries.length === 0 && <p className="muted">No entries yet.</p>}
      {entries.map((e) => <ThreadEntry key={e.id} entry={e} />)}
      {q.hasNextPage && <button type="button" disabled={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>Load more</button>}
    </section>
  )
}
