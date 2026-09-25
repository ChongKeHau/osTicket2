import { useMutation, useQueryClient } from '@tanstack/react-query'
import { assign, setStatus, transfer, updateTicket } from '../api/tickets'
import type { UpdateTicketInput } from '../api/types'

export const ticketQueryKey = (id: number) => ['ticket', id] as const

export function useInvalidateTicket(id: number) {
  const qc = useQueryClient()
  return async () => {
    await Promise.all([
      qc.invalidateQueries({ queryKey: ['ticket', id] }),
      qc.invalidateQueries({ queryKey: ['thread', id] }),
      qc.invalidateQueries({ queryKey: ['events', id] }),
      qc.invalidateQueries({ queryKey: ['tickets'] }),
    ])
  }
}

export function useTicketMutations(id: number) {
  const invalidate = useInvalidateTicket(id)
  return {
    setStatus: useMutation({ mutationFn: (statusId: number) => setStatus(id, statusId), onSuccess: invalidate }),
    assign: useMutation({ mutationFn: (staffId: number | null) => assign(id, staffId), onSuccess: invalidate }),
    transfer: useMutation({ mutationFn: (deptId: number) => transfer(id, deptId), onSuccess: invalidate }),
    update: useMutation({ mutationFn: (input: UpdateTicketInput) => updateTicket(id, input), onSuccess: invalidate }),
  }
}
