import { request } from './client'
import type { DashboardStats } from './types'

export function getDashboardStats(start: string, period: number): Promise<DashboardStats> {
  return request('GET', '/dashboard/stats', { query: { start, period } })
}
