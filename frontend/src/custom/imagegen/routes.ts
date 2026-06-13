/**
 * custom 生图路由定义。
 *
 * 主路由表后续只需要读取这里的路径和权限元数据，避免把 custom 页面路径散落到全局路由文件。
 */
export interface CustomImagegenRouteItem {
  path: string
  name: string
  label: string
  title: string
  requiresAuth: boolean
  requiresAdmin?: boolean
}

/**
 * 用户侧和管理员侧入口路径。
 */
export const customImagegenRoutes: CustomImagegenRouteItem[] = [
  {
    path: '/custom/images',
    name: 'CustomImageGeneration',
    label: 'AI 生图',
    title: 'AI 生图',
    requiresAuth: true,
  },
  {
    path: '/custom/images/history',
    name: 'CustomImageHistory',
    label: '我的图片',
    title: '我的图片',
    requiresAuth: true,
  },
  {
    path: '/custom/images/gallery',
    name: 'CustomImageGallery',
    label: '公共图库',
    title: '公共图库',
    requiresAuth: false,
  },
  {
    path: '/admin/custom/images',
    name: 'AdminCustomImages',
    label: '生图配置',
    title: '生图配置',
    requiresAuth: true,
    requiresAdmin: true,
  },
]

/**
 * 按路由名查找 custom 生图入口。
 */
export function findCustomImagegenRoute(name: string): CustomImagegenRouteItem | undefined {
  return customImagegenRoutes.find((route) => route.name === name)
}
