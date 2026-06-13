/**
 * custom 模型广场路由声明。
 *
 * 主路由和侧边栏只从这里读取路径，避免二开页面路径散落在主仓入口文件里。
 */
export interface CustomModelMarketplaceRouteItem {
  path: string
  name: string
  label: string
  title: string
  requiresAuth: boolean
}

/**
 * 用户侧模型广场入口。
 */
export const customModelMarketplaceRoutes: CustomModelMarketplaceRouteItem[] = [
  {
    path: '/model-marketplace',
    name: 'ModelMarketplace',
    label: '模型广场',
    title: '模型广场',
    requiresAuth: true,
  },
]

/**
 * 按路由名查找模型广场入口。
 */
export function findCustomModelMarketplaceRoute(name: string): CustomModelMarketplaceRouteItem | undefined {
  return customModelMarketplaceRoutes.find((route) => route.name === name)
}
