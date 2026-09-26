import type { PendingFile } from './FileUpload'

/** The done files' ids and, index for index, their tokens (empty for staff uploads). */
export function doneFiles(pending: PendingFile[]): { ids: number[]; tokens: string[] } {
  const done = pending.filter((p) => p.status === 'done' && p.fileId !== undefined)
  return { ids: done.map((p) => p.fileId as number), tokens: done.map((p) => p.token ?? '') }
}
