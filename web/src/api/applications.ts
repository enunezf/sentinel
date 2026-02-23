import { apiClient } from './client'
import type { Application, PaginatedResponse } from '@/types'

export interface ListApplicationsParams {
  page?: number
  page_size?: number
  search?: string
  is_active?: boolean
}

export interface CreateApplicationRequest {
  name: string
  slug: string
}

export interface UpdateApplicationRequest {
  name?: string
  is_active?: boolean
}

export const applicationsApi = {
  list(params: ListApplicationsParams = {}): Promise<PaginatedResponse<Application>> {
    return apiClient.get('/admin/applications', { params }).then((r) => r.data)
  },

  get(id: string): Promise<Application> {
    return apiClient.get(`/admin/applications/${id}`).then((r) => r.data)
  },

  create(data: CreateApplicationRequest): Promise<Application> {
    return apiClient.post('/admin/applications', data).then((r) => r.data)
  },

  update(id: string, data: UpdateApplicationRequest): Promise<Application> {
    return apiClient.put(`/admin/applications/${id}`, data).then((r) => r.data)
  },

  rotateKey(id: string): Promise<{ secret_key: string }> {
    return apiClient.post(`/admin/applications/${id}/rotate-key`).then((r) => r.data)
  },
}
