import {
  Bell,
  BookOpen,
  CircleHelp,
  LayoutDashboard,
  ListTodo,
  Monitor,
  Package,
  Palette,
  Route,
  Server,
  Settings,
  Users,
  Wrench,
  type LucideIcon,
} from 'lucide-react'

type SidebarGroupId = 'general' | 'pages' | 'other'
type SettingsSection = 'system' | 'profile'

export type Capability = `${string}:${string}`
export type CapabilityChecker = (capability: Capability) => boolean

export type FrontendModuleId =
  | 'dashboard'
  | 'tasks'
  | 'apps'
  | 'users'
  | 'wiki'
  | 'server-status'
  | 'settings'
  | 'settings-proxy'
  | 'settings-servers'
  | 'settings-help-center'
  | 'settings-account'
  | 'settings-appearance'
  | 'settings-notifications'
  | 'settings-display'
  | 'help-center'

export type FrontendModuleManifest = {
  id: FrontendModuleId
  title: string
  path: string
  icon: LucideIcon
  sidebar?: {
    groupId: SidebarGroupId
    order: number
    parentId?: FrontendModuleId
  }
  command?: {
    enabled: boolean
  }
  settings?: {
    section: SettingsSection
    order: number
  }
  permissions?: Capability[]
}

export type ModuleNavigationItem = {
  title: string
  path: string
  icon: LucideIcon
  children?: ModuleNavigationItem[]
}

export type ModuleNavigationGroup = {
  title: string
  items: ModuleNavigationItem[]
}

export type SettingsNavigationItem = {
  title: string
  href: string
  icon: LucideIcon
}

const sidebarGroupTitles: Record<SidebarGroupId, string> = {
  general: '通用',
  pages: '页面',
  other: '其他',
}

const sidebarGroupOrder: SidebarGroupId[] = ['general', 'pages', 'other']

export const frontendModuleManifests: FrontendModuleManifest[] = [
  {
    id: 'dashboard',
    title: '仪表盘',
    path: '/',
    icon: LayoutDashboard,
    permissions: ['dashboard:read'],
    sidebar: { groupId: 'general', order: 10 },
    command: { enabled: true },
  },
  {
    id: 'tasks',
    title: '任务',
    path: '/tasks',
    icon: ListTodo,
    permissions: ['tasks:read'],
    sidebar: { groupId: 'general', order: 20 },
    command: { enabled: true },
  },
  {
    id: 'apps',
    title: '应用',
    path: '/apps',
    icon: Package,
    permissions: ['apps:read'],
    sidebar: { groupId: 'general', order: 30 },
    command: { enabled: true },
  },
  {
    id: 'users',
    title: '用户',
    path: '/users',
    icon: Users,
    permissions: ['users:read'],
    sidebar: { groupId: 'general', order: 40 },
    command: { enabled: true },
  },
  {
    id: 'wiki',
    title: '资料库',
    path: '/wiki',
    icon: BookOpen,
    permissions: ['wiki:read'],
    sidebar: { groupId: 'general', order: 50 },
    command: { enabled: true },
  },
  {
    id: 'server-status',
    title: '服务器状态',
    path: '/server-status',
    icon: Server,
    permissions: ['servers:read'],
    sidebar: { groupId: 'pages', order: 10 },
    command: { enabled: true },
  },
  {
    id: 'settings',
    title: '设置',
    path: '/settings',
    icon: Settings,
    sidebar: { groupId: 'other', order: 10 },
    command: { enabled: false },
  },
  {
    id: 'settings-proxy',
    title: '代理',
    path: '/settings/proxy',
    icon: Route,
    permissions: ['proxy:read'],
    sidebar: { groupId: 'other', parentId: 'settings', order: 10 },
    command: { enabled: true },
    settings: { section: 'system', order: 10 },
  },
  {
    id: 'settings-servers',
    title: '服务器',
    path: '/settings/servers',
    icon: Server,
    permissions: ['servers:read'],
    sidebar: { groupId: 'other', parentId: 'settings', order: 20 },
    command: { enabled: true },
    settings: { section: 'system', order: 20 },
  },
  {
    id: 'settings-help-center',
    title: '帮助中心',
    path: '/settings/help-center',
    icon: CircleHelp,
    sidebar: { groupId: 'other', parentId: 'settings', order: 40 },
    command: { enabled: true },
    settings: { section: 'system', order: 40 },
  },
  {
    id: 'settings-account',
    title: '账号',
    path: '/settings/account',
    icon: Wrench,
    command: { enabled: true },
    settings: { section: 'profile', order: 10 },
  },
  {
    id: 'settings-appearance',
    title: '外观',
    path: '/settings/appearance',
    icon: Palette,
    command: { enabled: true },
    settings: { section: 'profile', order: 20 },
  },
  {
    id: 'settings-notifications',
    title: '通知',
    path: '/settings/notifications',
    icon: Bell,
    permissions: ['notifications:read'],
    sidebar: { groupId: 'other', parentId: 'settings', order: 30 },
    command: { enabled: true },
    settings: { section: 'system', order: 30 },
  },
  {
    id: 'settings-display',
    title: '显示',
    path: '/settings/display',
    icon: Monitor,
    command: { enabled: true },
    settings: { section: 'profile', order: 40 },
  },
  {
    id: 'help-center',
    title: '帮助中心',
    path: '/help-center',
    icon: CircleHelp,
    sidebar: { groupId: 'other', order: 20 },
    command: { enabled: true },
  },
]

export function getSidebarNavGroups(
  can?: CapabilityChecker
): ModuleNavigationGroup[] {
  const sidebarModules = frontendModuleManifests
    .filter((module) => module.sidebar)
    .sort((a, b) => a.sidebar!.order - b.sidebar!.order)

  return sidebarGroupOrder
    .map((groupId) => {
      const groupModules = sidebarModules.filter(
        (module) => module.sidebar?.groupId === groupId
      )
      const parentModules = groupModules.filter(
        (module) => !module.sidebar?.parentId && isModuleVisible(module, can)
      )

      return {
        title: sidebarGroupTitles[groupId],
        items: parentModules.map((module) => {
          const children = groupModules
            .filter(
              (child) =>
                child.sidebar?.parentId === module.id &&
                isModuleVisible(child, can)
            )
            .map(toNavigationItem)

          return {
            ...toNavigationItem(module),
            children: children.length ? children : undefined,
          }
        }),
      }
    })
    .filter((group) => group.items.length > 0)
}

export function getCommandNavGroups(
  can?: CapabilityChecker
): ModuleNavigationGroup[] {
  return getSidebarNavGroups(can)
}

export function getSettingsNavItems(
  section: SettingsSection,
  can?: CapabilityChecker
): SettingsNavigationItem[] {
  return frontendModuleManifests
    .filter(
      (module) =>
        module.settings?.section === section && isModuleVisible(module, can)
    )
    .sort((a, b) => a.settings!.order - b.settings!.order)
    .map((module) => ({
      title: module.title,
      href: module.path,
      icon: module.icon,
    }))
}

export function isSystemSettingsPath(pathname: string) {
  return frontendModuleManifests.some(
    (module) =>
      module.settings?.section === 'system' && module.path === pathname
  )
}

export function getModuleForPath(pathname: string) {
  return [...frontendModuleManifests]
    .sort((a, b) => b.path.length - a.path.length)
    .find((module) => {
      if (module.path === '/') {
        return pathname === '/'
      }
      return pathname === module.path || pathname.startsWith(`${module.path}/`)
    })
}

export function canAccessModule(
  module: FrontendModuleManifest | undefined,
  can: CapabilityChecker
) {
  if (!module) {
    return true
  }
  return isModuleVisible(module, can)
}

function toNavigationItem(
  module: FrontendModuleManifest
): ModuleNavigationItem {
  return {
    title: module.title,
    path: module.path,
    icon: module.icon,
  }
}

function isModuleVisible(
  module: FrontendModuleManifest,
  can?: CapabilityChecker
) {
  if (!module.permissions?.length || !can) {
    return true
  }
  return module.permissions.every((permission) => can(permission))
}
