import { useState } from 'react'

/**
 * The live record for `id`, or the last one seen for that id once it drops out of the list
 * (e.g. deleted elsewhere and refetched after a failed save), so a mounted form can keep
 * the user's edits and show the error instead of flipping to "not found".
 */
export function useLastSeen<T extends { id: number }>(live: T | undefined, id: number | null): T | undefined {
  const [seen, setSeen] = useState(live)
  if (live && live !== seen) setSeen(live)
  return live ?? (seen && seen.id === id ? seen : undefined)
}
