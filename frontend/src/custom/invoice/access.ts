import { ref } from 'vue'
import { getInvoiceAccess } from './api'

const canManageInvoice = ref(false)
const accessLoaded = ref(false)
let accessRequest: Promise<boolean> | null = null

/** Shared sidebar state for the current user's delegated invoice access. */
export const invoiceAccessState = {
  canManage: canManageInvoice,
  loaded: accessLoaded,
}

/**
 * Refreshes the current user's invoice-management access.
 * Failed reads remain unloaded so callers can report the error and retry later.
 */
export async function refreshInvoiceAccess(force = false): Promise<boolean> {
  if (!force && accessLoaded.value) {
    return canManageInvoice.value
  }
  if (accessRequest) {
    return accessRequest
  }
  accessRequest = getInvoiceAccess()
    .then((access) => {
      canManageInvoice.value = access.can_manage === true
      accessLoaded.value = true
      return canManageInvoice.value
    })
    .catch((error) => {
      canManageInvoice.value = false
      accessLoaded.value = false
      throw error
    })
    .finally(() => {
      accessRequest = null
    })
  return accessRequest
}

/** Clears access state when the authenticated user changes or signs out. */
export function clearInvoiceAccess(): void {
  canManageInvoice.value = false
  accessLoaded.value = false
}
