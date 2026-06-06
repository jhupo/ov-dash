import { useQuery } from '@tanstack/react-query'
import { ConfigDrawer } from '@/components/config-drawer'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'
import { TasksDialogs } from './components/tasks-dialogs'
import { TasksPrimaryButtons } from './components/tasks-primary-buttons'
import { TasksProvider } from './components/tasks-provider'
import { TasksTable } from './components/tasks-table'
import { getTasks } from '@/services/tasks'

export function Tasks() {
  const tasks = useQuery({
    queryKey: ['tasks'],
    queryFn: getTasks,
    refetchInterval: 5000,
  })

  return (
    <TasksProvider>
      <Header fixed>
        <Search className='me-auto' />
        <ThemeSwitch />
        <ConfigDrawer />
        <ProfileDropdown />
      </Header>

      <Main className='flex flex-1 flex-col gap-4 sm:gap-6'>
        <div className='flex flex-wrap items-end justify-between gap-2'>
          <div>
            <h2 className='text-2xl font-bold tracking-tight'>任务</h2>
            <p className='text-muted-foreground'>
              查看后台事件队列和采集任务状态。
            </p>
          </div>
          <TasksPrimaryButtons />
        </div>
        <TasksTable data={tasks.data ?? []} />
      </Main>

      <TasksDialogs />
    </TasksProvider>
  )
}
