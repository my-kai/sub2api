import { formatCurrency, formatDateTime } from '@/utils/format'
import type { InvoiceApplicationStatus } from './types'

/**
 * Converts backend invoice status into concise product copy.
 *
 * @param status - Backend invoice application status.
 * @returns User-visible status label.
 */
export function invoiceStatusLabel(status: InvoiceApplicationStatus | string): string {
  const labels: Record<string, string> = {
    pending: '待审核',
    issued: '已开票',
    rejected: '已驳回',
  }
  return labels[status] || status
}

/**
 * Keeps invoice status badge colors consistent across user and admin pages.
 *
 * @param status - Backend invoice application status.
 * @returns Tailwind class list for the badge.
 */
export function invoiceStatusClass(status: InvoiceApplicationStatus | string): string {
  if (status === 'issued') return 'bg-green-50 text-green-700 dark:bg-green-500/10 dark:text-green-300'
  if (status === 'rejected') return 'bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-300'
  return 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300'
}

/**
 * Formats invoice amount using the project currency formatter.
 *
 * @param value - Decimal string returned by backend.
 * @param currency - ISO currency code.
 * @returns Localized currency amount.
 */
export function formatInvoiceAmount(value: string | number | null | undefined, currency = 'CNY'): string {
  const amount = Number(value || 0)
  return formatCurrency(Number.isFinite(amount) ? amount : 0, currency)
}

/**
 * Formats invoice datetime and keeps empty values visible as "-".
 *
 * @param value - ISO datetime string.
 * @returns Localized datetime or "-".
 */
export function formatInvoiceDate(value?: string | null): string {
  return value ? formatDateTime(value) || '-' : '-'
}

/**
 * Formats PDF file size for admin detail display.
 *
 * @param bytes - File size in bytes.
 * @returns Human-readable size.
 */
export function formatInvoiceFileSize(bytes?: number): string {
  if (!bytes || bytes <= 0) return '-'
  const mb = bytes / 1024 / 1024
  return `${mb.toFixed(2)} MB`
}

/**
 * Saves a downloaded invoice PDF blob without exposing auth tokens in links.
 *
 * @param blob - PDF blob returned by the API client.
 * @param fileName - Suggested local filename.
 */
export function saveInvoiceBlob(blob: Blob, fileName: string): void {
  const url = window.URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = fileName || 'invoice.pdf'
  document.body.appendChild(link)
  link.click()
  link.remove()
  window.URL.revokeObjectURL(url)
}
