import { useQueryClient } from '@tanstack/react-query'
import { Banner, errorMessage } from '../../ui/Banner'
import { Button } from '../../ui/Button'

/** Error banner for a failed reference-data load, with a Retry that refetches every `ref` query. */
export function RefDataError({ error }: { error: unknown }) {
  const qc = useQueryClient()
  return (
    <Banner level="error">
      {errorMessage(error)}{' '}
      <Button size="sm" onClick={() => void qc.refetchQueries({ queryKey: ['ref'] })}>Retry</Button>
    </Banner>
  )
}
