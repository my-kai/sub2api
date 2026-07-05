import type { RouteRecordRaw } from 'vue-router'

/**
 * custom 开票路由元数据。
 *
 * 主路由和侧边栏从这里读取路径，避免开票入口散落在全局文件里。
 */
export interface CustomInvoiceRouteItem {
  path: string
  name: string
  label: string
  title: string
  requiresAuth: boolean
  requiresAdmin?: boolean
}

export const customInvoiceRoutes: CustomInvoiceRouteItem[] = [
  {
    path: '/invoices',
    name: 'UserInvoiceApplications',
    label: '开票申请',
    title: '开票申请',
    requiresAuth: true,
  },
  {
    path: '/invoice-titles',
    name: 'UserInvoiceTitles',
    label: '抬头管理',
    title: '抬头管理',
    requiresAuth: true,
  },
  {
    path: '/admin/custom/invoices',
    name: 'AdminInvoiceApplications',
    label: '开票管理',
    title: '开票管理',
    requiresAuth: true,
    requiresAdmin: true,
  },
]

/**
 * 可直接并入主路由表的开票页面记录。
 */
export const customInvoiceRouteRecords: RouteRecordRaw[] = [
  {
    path: '/invoices',
    name: 'UserInvoiceApplications',
    component: () => import('./views/UserInvoiceApplicationsView.vue'),
    meta: {
      title: '开票申请',
      requiresAuth: true,
      requiresAdmin: false,
    },
  },
  {
    path: '/invoice-titles',
    name: 'UserInvoiceTitles',
    component: () => import('./views/UserInvoiceTitlesView.vue'),
    meta: {
      title: '抬头管理',
      requiresAuth: true,
      requiresAdmin: false,
    },
  },
  {
    path: '/admin/custom/invoices',
    name: 'AdminInvoiceApplications',
    component: () => import('./views/AdminInvoiceApplicationsView.vue'),
    meta: {
      title: '开票管理',
      requiresAuth: true,
      requiresAdmin: true,
    },
  },
]

/**
 * 按路由名查找 custom 开票入口。
 *
 * @param name - custom 开票路由名。
 * @returns 命中的路由声明；不存在时返回 undefined。
 */
export function findCustomInvoiceRoute(name: string): CustomInvoiceRouteItem | undefined {
  return customInvoiceRoutes.find((route) => route.name === name)
}
