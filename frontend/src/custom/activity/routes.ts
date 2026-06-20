import type { RouteLocationRaw, RouteRecordRaw } from 'vue-router'
import type { CustomActivityListItem } from './types'

/**
 * custom 活动中心路由元数据。
 *
 * 主路由后续只需要引用这里，避免活动页面路径散落在全局路由文件里。
 */
export interface CustomActivityRouteItem {
  path: string
  name: string
  label: string
  title: string
  requiresAuth: boolean
  requiresAdmin?: boolean
}

/**
 * 用户侧活动中心入口。
 */
export const customActivityUserRoutes: CustomActivityRouteItem[] = [
  {
    path: '/custom/activities',
    name: 'CustomActivityHall',
    label: '活动中心',
    title: '活动中心',
    requiresAuth: true,
  },
  {
    path: '/custom/activities/:id/red-packet-rain',
    name: 'CustomRedPacketRainDetail',
    label: '红包雨',
    title: '红包雨',
    requiresAuth: true,
  },
]

/**
 * 管理员侧活动管理入口。
 */
export const customActivityAdminRoutes: CustomActivityRouteItem[] = [
  {
    path: '/admin/custom/activities',
    name: 'AdminCustomActivities',
    label: '活动管理',
    title: '活动管理',
    requiresAuth: true,
    requiresAdmin: true,
  },
  {
    path: '/admin/custom/activities/:id',
    name: 'AdminCustomActivityDetail',
    label: '活动详情',
    title: '活动详情',
    requiresAuth: true,
    requiresAdmin: true,
  },
]

/**
 * 活动中心全部主入口，供 router/sidebar 统一按名称查找路径。
 */
export const customActivityRoutes: CustomActivityRouteItem[] = [
  ...customActivityUserRoutes,
  ...customActivityAdminRoutes,
]

/**
 * 可直接并入主路由表的活动页面记录。
 */
export const customActivityRouteRecords: RouteRecordRaw[] = [
  {
    path: '/custom/activities',
    name: 'CustomActivityHall',
    component: () => import('./views/ActivityHallView.vue'),
    meta: {
      title: '活动中心',
      requiresAuth: true,
      requiresAdmin: false,
    },
  },
  {
    path: '/custom/activities/:id/red-packet-rain',
    name: 'CustomRedPacketRainDetail',
    component: () => import('./views/RedPacketRainDetailView.vue'),
    meta: {
      title: '红包雨',
      requiresAuth: true,
      requiresAdmin: false,
    },
  },
  {
    path: '/admin/custom/activities',
    name: 'AdminCustomActivities',
    component: () => import('./views/AdminActivityManagementView.vue'),
    meta: {
      title: '活动管理',
      requiresAuth: true,
      requiresAdmin: true,
    },
  },
  {
    path: '/admin/custom/activities/:id',
    name: 'AdminCustomActivityDetail',
    component: () => import('./views/AdminActivityManagementView.vue'),
    meta: {
      title: '活动详情',
      requiresAuth: true,
      requiresAdmin: true,
    },
  },
]

/**
 * 按路由名查找活动中心入口。
 */
export function findCustomActivityRoute(name: string): CustomActivityRouteItem | undefined {
  return customActivityRoutes.find((route) => route.name === name)
}

/**
 * 根据活动类型生成用户详情页位置。
 *
 * @param activity - 活动大厅列表项。
 * @returns 当前活动类型对应的详情页位置。
 */
export function activityDetailRouteFor(activity: CustomActivityListItem): RouteLocationRaw {
  return {
    name: 'CustomRedPacketRainDetail',
    params: {
      id: String(activity.id),
    },
  }
}
