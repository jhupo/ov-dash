import {
  Bell,
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

export type FrontendModuleId =
  | 'dashboard'
  | 'tasks'
  | 'apps'
  | 'users'
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
    sidebar: { groupId: 'general', order: 10 },
    command: { enabled: true },
  },
  {
    id: 'tasks',
    title: '任务',
    path: '/tasks',
    icon: ListTodo,
    sidebar: { groupId: 'general', order: 20 },
    command: { enabled: true },
  },
  {
    id: 'apps',
    title: '应用',
    path: '/apps',
    icon: Package,
    sidebar: { groupId: 'general', order: 30 },
    command: { enabled: true },
  },
  {
    id: 'users',
    title: '用户',
    path: '/users',
    icon: Users,
    sidebar: { groupId: 'general', order: 40 },
    command: { enabled: true },
  },
  {
    id: 'server-status',
    title: '服务器状态',
    path: '/server-status',
    icon: Server,
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
    sidebar: { groupId: 'other', parentId: 'settings', order: 10 },
    command: { enabled: true },
    settings: { section: 'system', order: 10 },
  },
  {
    id: 'settings-servers',
    title: '服务器',
    path: '/settings/servers',
    icon: Server,
    sidebar: { groupId: 'other', parentId: 'settings', order: 20 },
    command: { enabled: true },
    settings: { section: 'system', order: 20 },
  },
  {
    id: 'settings-help-center',
    title: '帮助中心',
    path: '/settings/help-center',
    icon: CircleHelp,
    sidebar: { groupId: 'other', parentId: 'settings', order: 30 },
    command: { enabled: true },
    settings: { section: 'system', order: 30 },
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
    command: { enabled: true },
    settings: { section: 'profile', order: 30 },
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

export function getSidebarNavGroups(): ModuleNavigationGroup[] {
  const sidebarModules = frontendModuleManifests
    .filter((module) => module.sidebar)
    .sort((a, b) => a.sidebar!.order - b.sidebar!.order)

  return sidebarGroupOrder
    .map((groupId) => {
      const groupModules = sidebarModules.filter(
        (module) => module.sidebar?.groupId === groupId
      )
      const parentModules = groupModules.filter(
        (module) => !module.sidebar?.parentId
      )

      return {
        title: sidebarGroupTitles[groupId],
        items: parentModules.map((module) => {
          const children = groupModules
            .filter((child) => child.sidebar?.parentId === module.id)
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

export function getCommandNavGroups(): ModuleNavigationGroup[] {
  return getSidebarNavGroups()
}

export function getSettingsNavItems(
  section: SettingsSection
): SettingsNavigationItem[] {
  return frontendModuleManifests
    .filter((module) => module.settings?.section === section)
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

function toNavigationItem(module: FrontendModuleManifest): ModuleNavigationItem {
  return {
    title: module.title,
    path: module.path,
    icon: module.icon,
  }
}
