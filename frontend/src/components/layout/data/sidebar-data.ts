import {
  Bell,
  BookOpen,
  LayoutDashboard,
  ListTodo,
  HelpCircle,
  Package,
  Route,
  Server,
  Settings,
  Users,
} from 'lucide-react'
import { type SidebarData } from '../types'

export const sidebarData: SidebarData = {
  user: {
    name: 'satnaing',
    email: 'satnaingdev@gmail.com',
    avatar: '/avatars/shadcn.jpg',
  },
  navGroups: [
    {
      title: '通用',
      items: [
        {
          title: '仪表盘',
          url: '/',
          icon: LayoutDashboard,
        },
        {
          title: '任务',
          url: '/tasks',
          icon: ListTodo,
        },
        {
          title: '应用',
          url: '/apps',
          icon: Package,
        },
        {
          title: '用户',
          url: '/users',
          icon: Users,
        },
        {
          title: '资料库',
          url: '/wiki',
          icon: BookOpen,
        },
      ],
    },
    {
      title: '页面',
      items: [
        {
          title: '服务器状态',
          url: '/server-status',
          icon: Server,
        },
      ],
    },
    {
      title: '其他',
      items: [
        {
          title: '设置',
          icon: Settings,
          items: [
            {
              title: '代理',
              url: '/settings/proxy',
              icon: Route,
            },
            {
              title: '服务器',
              url: '/settings/servers',
              icon: Server,
            },
            {
              title: '通知',
              url: '/settings/notifications',
              icon: Bell,
            },
            {
              title: '帮助中心',
              url: '/settings/help-center',
              icon: HelpCircle,
            },
          ],
        },
        {
          title: '帮助中心',
          url: '/help-center',
          icon: HelpCircle,
        },
      ],
    },
  ],
}
