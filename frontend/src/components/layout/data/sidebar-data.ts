import { getSidebarNavGroups } from '@/services/module-registry'
import { type NavItem, type SidebarData } from '../types'

export const sidebarData: SidebarData = {
  user: {
    name: 'satnaing',
    email: 'satnaingdev@gmail.com',
    avatar: '/avatars/shadcn.jpg',
  },
  navGroups: getSidebarNavGroups().map((group) => ({
    title: group.title,
    items: group.items.map((item): NavItem => {
      if (item.children?.length) {
        return {
          title: item.title,
          icon: item.icon,
          items: item.children.map((child) => ({
            title: child.title,
            url: child.path,
            icon: child.icon,
          })),
        }
      }

      return {
        title: item.title,
        url: item.path,
        icon: item.icon,
      }
    }),
  })),
}
