import type { RouteRecordRaw } from 'vue-router'

/**
 * 自定义 OAuth 应用路由声明。
 *
 * 主路由和侧边栏只从这里读取路径，避免第三方应用管理入口散落在主仓文件里。
 */
export interface CustomOAuthAppRouteItem {
  path: string
  name: string
  label: string
  title: string
  requiresAuth: boolean
  requiresAdmin?: boolean
}

export const customOAuthAppRoutes: CustomOAuthAppRouteItem[] = [
  {
    path: '/admin/custom/oauth-applications',
    name: 'AdminCustomOAuthApplications',
    label: '应用管理',
    title: '应用管理',
    requiresAuth: true,
    requiresAdmin: true,
  },
  {
    path: '/auth/oauth/authorize',
    name: 'CustomOAuthAuthorize',
    label: '应用授权',
    title: '应用授权',
    requiresAuth: true,
  },
]

export const customOAuthAppRouteRecords: RouteRecordRaw[] = [
  {
    path: '/admin/custom/oauth-applications',
    name: 'AdminCustomOAuthApplications',
    component: () => import('./AdminOAuthApplicationsView.vue'),
    meta: {
      title: '应用管理',
      requiresAuth: true,
      requiresAdmin: true,
    },
  },
  {
    path: '/auth/oauth/authorize',
    name: 'CustomOAuthAuthorize',
    component: () => import('./OAuthAuthorizeView.vue'),
    meta: {
      title: '应用授权',
      requiresAuth: true,
      requiresAdmin: false,
    },
  },
]

/**
 * 判断当前路径是否为第三方 OAuth 授权确认页。
 *
 * @param path - vue-router 解析后的站内路径
 * @returns 命中自定义 OAuth 授权页时返回 true
 */
export function isCustomOAuthAuthorizeRoute(path: string): boolean {
  return path === '/auth/oauth/authorize'
}

/**
 * 按路由名查找 custom OAuth 应用入口。
 */
export function findCustomOAuthAppRoute(name: string): CustomOAuthAppRouteItem | undefined {
  return customOAuthAppRoutes.find((route) => route.name === name)
}
