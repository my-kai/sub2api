import type { RouteRecordRaw } from 'vue-router'

/** Custom route records kept outside the upstream router implementation. */
export const promptAuditV2RouteRecords: RouteRecordRaw[] = [
  {
    path: '/admin/prompt-audit-v2',
    name: 'AdminPromptAuditV2',
    component: () => import('./PromptAuditV2View.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: true,
      requiresRiskControl: true,
      title: 'Prompt Audit 2',
      titleKey: 'admin.promptAuditV2.title',
      descriptionKey: 'admin.promptAuditV2.description',
    },
  },
]
