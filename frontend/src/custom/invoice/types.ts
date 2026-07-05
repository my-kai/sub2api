import type { BasePaginationResponse } from '@/types'

export type InvoiceApplicationStatus = 'pending' | 'issued' | 'rejected'

export interface InvoiceTitle {
  id: number
  user_id: number
  company_title: string
  tax_number: string
  receiver_email: string
  is_default: boolean
  created_at: string
  updated_at: string
}

export interface InvoiceTitlePayload {
  company_title: string
  tax_number: string
  receiver_email: string
  is_default: boolean
}

export interface EligibleInvoiceOrder {
  id: number
  out_trade_no: string
  amount: string
  pay_amount: string
  currency: string
  payment_type: string
  status: string
  paid_at?: string
  completed_at?: string
  created_at: string
}

export interface InvoiceApplicationOrder {
  application_id: number
  order_id: number
  user_id: number
  amount: string
  currency: string
  out_trade_no?: string
  payment_type?: string
  status?: string
  paid_at?: string
  completed_at?: string
  created_at: string
}

export interface InvoiceApplication {
  id: number
  application_no: string
  user_id: number
  status: InvoiceApplicationStatus
  invoice_type: string
  title_id?: number
  company_title: string
  tax_number: string
  receiver_email: string
  total_amount: string
  currency: string
  order_count: number
  invoice_number: string
  admin_remark: string
  reject_reason: string
  file_original_name?: string
  file_size?: number
  issued_by?: number
  issued_at?: string
  rejected_by?: number
  rejected_at?: string
  created_at: string
  updated_at: string
  orders?: InvoiceApplicationOrder[]
}

export interface CreateInvoiceApplicationPayload {
  order_ids: number[]
  title_id: number
}

export interface RejectInvoiceApplicationPayload {
  reason: string
}

export type InvoiceApplicationPage = BasePaginationResponse<InvoiceApplication>
