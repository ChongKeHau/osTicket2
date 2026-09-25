import { request } from './client'
import type { Department, Priority, Staff, Status, Topic } from './types'

async function items<T>(path: string): Promise<T[]> { return (await request<{ items: T[] }>('GET', path)).items }
export const listPriorities = () => items<Priority>('/priorities')
export const listStatuses = () => items<Status>('/statuses')
export const listDepartments = () => items<Department>('/departments')
export const listTopics = () => items<Topic>('/topics')
export const listStaff = () => items<Staff>('/staff')
