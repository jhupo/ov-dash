import { Outlet, useLocation } from '@tanstack/react-router'
import {
  Bell,
  CircleHelp,
  Monitor,
  Palette,
  Route,
  Server,
  Wrench,
} from 'lucide-react'
import { ConfigDrawer } from '@/components/config-drawer'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'
import { SidebarNav } from './components/sidebar-nav'

const systemNavItems = [
  {
    title: '代理',
    href: '/settings/proxy',
    icon: <Route size={18} />,
  },
  {
    title: '服务器',
    href: '/settings/servers',
    icon: <Server size={18} />,
  },
  {
    title: '帮助中心',
    href: '/settings/help-center',
    icon: <CircleHelp size={18} />,
  },
]

const profileNavItems = [
  {
    title: '账号',
    href: '/settings/account',
    icon: <Wrench size={18} />,
  },
  {
    title: '外观',
    href: '/settings/appearance',
    icon: <Palette size={18} />,
  },
  {
    title: '通知',
    href: '/settings/notifications',
    icon: <Bell size={18} />,
  },
  {
    title: '显示',
    href: '/settings/display',
    icon: <Monitor size={18} />,
  },
]

export function Settings() {
  const pathname = useLocation({ select: (location) => location.pathname })
  const isSystemSettings =
    pathname === '/settings/proxy' ||
    pathname === '/settings/servers' ||
    pathname === '/settings/help-center'

  return (
    <>
      {/* ===== Top Heading ===== */}
      <Header>
        <Search className='me-auto' />
        <ThemeSwitch />
        <ConfigDrawer />
        <ProfileDropdown />
      </Header>

      <Main fixed>
        <div className='flex flex-1 flex-col gap-4 overflow-hidden'>
          <SidebarNav
            items={isSystemSettings ? systemNavItems : profileNavItems}
          />
          <div className='flex w-full flex-1 overflow-y-hidden p-1'>
            <Outlet />
          </div>
        </div>
      </Main>
    </>
  )
}
