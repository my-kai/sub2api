import type { RouteRecordRaw } from 'vue-router'

/** Admin-only route records for the custom turn-log page. */
export const turnLogRouteRecords: RouteRecordRaw[] = [
  {
    path: '/admin/custom/turn-logs',
    name: 'AdminTurnLogs',
    component: () => import('./views/TurnLogView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: true,
      title: 'turn记录',
      titleKey: 'admin.turnLog.title',
      descriptionKey: 'admin.turnLog.description',
    },
  },
]
