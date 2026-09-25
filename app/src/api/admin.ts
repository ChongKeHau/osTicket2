import { request } from './client'
import type { CreateStaffInput, Department, DepartmentInput, Staff, Topic, TopicInput, UpdateStaffInput } from './types'

export function createDepartment(input: DepartmentInput): Promise<Department> { return request('POST', '/departments', { body: input }) }
export function updateDepartment(id: number, input: Partial<DepartmentInput>): Promise<Department> { return request('PATCH', `/departments/${id}`, { body: input }) }
export function deleteDepartment(id: number): Promise<void> { return request('DELETE', `/departments/${id}`) }

export function createTopic(input: TopicInput): Promise<Topic> { return request('POST', '/topics', { body: input }) }
export function updateTopic(id: number, input: Partial<TopicInput>): Promise<Topic> { return request('PATCH', `/topics/${id}`, { body: input }) }
export function deleteTopic(id: number): Promise<void> { return request('DELETE', `/topics/${id}`) }

export function createStaff(input: CreateStaffInput): Promise<Staff> { return request('POST', '/staff', { body: input }) }
export function updateStaff(id: number, input: UpdateStaffInput): Promise<Staff> { return request('PATCH', `/staff/${id}`, { body: input }) }
export function setStaffPassword(id: number, password: string): Promise<void> { return request('POST', `/staff/${id}/password`, { body: { password } }) }
