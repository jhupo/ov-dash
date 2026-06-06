import { Outlet, useLocation } from '@tanstack/react-router'
import { Monitor, Palette, Route } from 'lucide-react'
import { Separator } from '@/components/ui/separator'
import { ConfigDrawer } from '@/components/config-drawer'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'
import { SidebarNav } from './components/sidebar-nav'

const sidebarNavItems = [
  {
    title: '外观',
    href: '/settings/appearance',
    icon: <Palette size={18} />,
  },
  {
    title: '代理',
    href: '/settings/proxy',
    icon: <Route size={18} />,
  },
  {
    title: '显示',
    href: '/settings/display',
    icon: <Monitor size={18} />,
  },
]

export function Settings() {
  const pathname = useLocation({ select: (location) => location.pathname })
  const isProfileSettings =
    pathname === '/settings/account' || pathname === '/settings/notifications'

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
            {isProfileSettings ? '个人设置' : '系统设置'}
          </h1>
          <p className='text-muted-foreground'>
            {isProfileSettings
              ? '管理个人资料和账号偏好。'
              : '管理系统外观、代理和显示配置。'}
          </p>
        </div>
        <Separator className='my-4 lg:my-6' />
        {isProfileSettings ? (
          <div className='flex w-full max-w-xl flex-1 overflow-y-hidden p-1'>
            <Outlet />
          </div>
        ) : (
          <div className='flex flex-1 flex-col space-y-2 overflow-hidden md:space-y-2 lg:flex-row lg:space-y-0 lg:space-x-12'>
            <aside className='top-0 lg:sticky lg:w-1/5'>
              <SidebarNav items={sidebarNavItems} />
            </aside>
            <div className='flex w-full overflow-y-hidden p-1'>
              <Outlet />
            </div>
          </div>
        )}
      </Main>
    </>
  )
}
