import { apiClient } from './client'
import type { CostCenter, PaginatedResponse } from '@/types'

export interface ListCostCentersParams {
  page?: number
  page_size?: number
  application_id?: string
  search?: string
}

export interface CreateCostCenterPayload {
  code: string
  name: string
}

export interface UpdateCostCenterPayload {
  name?: string
  is_active?: boolean
}

export const costCentersApi = {
  list: (params: ListCostCentersParams = {}) =>
    apiClient.get<PaginatedResponse<CostCenter>>('/admin/cost-centers', { params }).then((r) => r.data),

  create: (payload: CreateCostCenterPayload) =>
    apiClient.post<CostCenter>('/admin/cost-centers', payload).then((r) => r.data),

  update: (id: string, payload: UpdateCostCenterPayload) =>
    apiClient.put<CostCenter>(`/admin/cost-centers/${id}`, payload).then((r) => r.data),
}
