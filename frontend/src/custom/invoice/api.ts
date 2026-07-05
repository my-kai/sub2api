import { apiClient } from '@/api/client'
import type {
  CreateInvoiceApplicationPayload,
  EligibleInvoiceOrder,
  InvoiceApplication,
  InvoiceApplicationPage,
  InvoiceTitle,
  InvoiceTitlePayload,
  RejectInvoiceApplicationPayload,
} from './types'

const USER_PREFIX = '/custom/invoices'
const ADMIN_PREFIX = '/admin/custom/invoices'

export async function listInvoiceTitles(): Promise<InvoiceTitle[]> {
  const { data } = await apiClient.get<InvoiceTitle[]>(`${USER_PREFIX}/titles`)
  return data
}

export async function createInvoiceTitle(payload: InvoiceTitlePayload): Promise<InvoiceTitle> {
  const { data } = await apiClient.post<InvoiceTitle>(`${USER_PREFIX}/titles`, payload)
  return data
}

export async function updateInvoiceTitle(id: number, payload: InvoiceTitlePayload): Promise<InvoiceTitle> {
  const { data } = await apiClient.put<InvoiceTitle>(`${USER_PREFIX}/titles/${id}`, payload)
  return data
}

export async function deleteInvoiceTitle(id: number): Promise<void> {
  await apiClient.delete(`${USER_PREFIX}/titles/${id}`)
}

export async function setDefaultInvoiceTitle(id: number): Promise<InvoiceTitle> {
  const { data } = await apiClient.post<InvoiceTitle>(`${USER_PREFIX}/titles/${id}/default`)
  return data
}

export async function listEligibleInvoiceOrders(): Promise<EligibleInvoiceOrder[]> {
  const { data } = await apiClient.get<{ items: EligibleInvoiceOrder[] }>(`${USER_PREFIX}/eligible-orders`)
  return data.items || []
}

export async function createInvoiceApplication(payload: CreateInvoiceApplicationPayload): Promise<InvoiceApplication> {
  const { data } = await apiClient.post<InvoiceApplication>(USER_PREFIX, payload)
  return data
}

export async function listMyInvoiceApplications(params: { page?: number; page_size?: number; status?: string }): Promise<InvoiceApplicationPage> {
  const { data } = await apiClient.get<InvoiceApplicationPage>(`${USER_PREFIX}/my`, { params })
  return data
}

export async function getMyInvoiceApplication(id: number): Promise<InvoiceApplication> {
  const { data } = await apiClient.get<InvoiceApplication>(`${USER_PREFIX}/${id}`)
  return data
}

export async function listAdminInvoiceApplications(params: { page?: number; page_size?: number; status?: string; user_id?: number }): Promise<InvoiceApplicationPage> {
  const { data } = await apiClient.get<InvoiceApplicationPage>(ADMIN_PREFIX, { params })
  return data
}

export async function getAdminInvoiceApplication(id: number): Promise<InvoiceApplication> {
  const { data } = await apiClient.get<InvoiceApplication>(`${ADMIN_PREFIX}/${id}`)
  return data
}

export async function issueInvoiceApplication(id: number, payload: { invoice_number: string; admin_remark: string; file: File }): Promise<InvoiceApplication> {
  const form = new FormData()
  form.append('invoice_number', payload.invoice_number)
  form.append('admin_remark', payload.admin_remark)
  form.append('file', payload.file)
  const { data } = await apiClient.post<InvoiceApplication>(`${ADMIN_PREFIX}/${id}/issue`, form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return data
}

export async function testSendAdminInvoiceEmail(id: number): Promise<void> {
  await apiClient.post(`${ADMIN_PREFIX}/${id}/test-email`)
}

export async function testSendGeneratedAdminInvoiceEmail(payload: { receiver_email: string }): Promise<void> {
  await apiClient.post('/admin/custom/invoice-test-email', payload)
}

export async function rejectInvoiceApplication(id: number, payload: RejectInvoiceApplicationPayload): Promise<InvoiceApplication> {
  const { data } = await apiClient.post<InvoiceApplication>(`${ADMIN_PREFIX}/${id}/reject`, payload)
  return data
}

export async function downloadUserInvoiceFile(id: number): Promise<Blob> {
  const { data } = await apiClient.get<Blob>(`${USER_PREFIX}/${id}/file`, { responseType: 'blob' })
  return data
}

export async function downloadAdminInvoiceFile(id: number): Promise<Blob> {
  const { data } = await apiClient.get<Blob>(`${ADMIN_PREFIX}/${id}/file`, { responseType: 'blob' })
  return data
}
