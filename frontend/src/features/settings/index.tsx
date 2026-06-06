import { Outlet, useLocation } from '@tanstack/react-router'
import { Bell, CircleHelp, Monitor, Palette, Route, Wrench } from 'lucide-react'
import { Separator } from '@/components/ui/separator'
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
  {
    title: '帮助中心',
    href: '/settings/help-center',
    icon: <CircleHelp size={18} />,
  },
]

export function Settings() {
  const pathname = useLocation({ select: (location) => location.pathname })
  const isSystemSettings = pathname === '/settings/proxy'

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
        <div className='space-y-0.5'>
          <h1 className='text-2xl font-bold tracking-tight md:text-3xl'>
            {isSystemSettings ? '系统设置' : '个人设置'}
          </h1>
          <p className='text-muted-foreground'>
            {isSystemSettings
              ? '管理系统代理配置。'
              : '管理账号、外观、通知和显示偏好。'}
          </p>
        </div>
        <Separator className='my-4 lg:my-6' />
        <div className='flex flex-1 flex-col space-y-2 overflow-hidden md:space-y-2 lg:flex-row lg:space-y-0 lg:space-x-12'>
          <aside className='top-0 lg:sticky lg:w-1/5'>
            <SidebarNav
              items={isSystemSettings ? systemNavItems : profileNavItems}
            />
          </aside>
          <div className='flex w-full overflow-y-hidden p-1'>
            <Outlet />
          </div>
        </div>
      </Main>
    </>
  )
}
