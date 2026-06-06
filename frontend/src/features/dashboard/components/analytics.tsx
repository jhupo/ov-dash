import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { type DashboardSnapshot } from '@/services/dashboard'
import { AnalyticsChart } from './analytics-chart'

type AnalyticsProps = {
  data: DashboardSnapshot['analytics']
}

function formatDuration(seconds: number) {
  const minutes = Math.floor(seconds / 60)
  const remainingSeconds = seconds % 60
  return `${minutes}分${remainingSeconds}秒`
}

export function Analytics({ data }: AnalyticsProps) {
  return (
    <div className='space-y-4'>
      <Card>
        <CardHeader>
          <CardTitle>流量概览</CardTitle>
          <CardDescription>每周点击量和独立访客</CardDescription>
        </CardHeader>
        <CardContent className='px-6'>
          <AnalyticsChart data={data.traffic} />
        </CardContent>
      </Card>
      <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
        <MetricCard
          title='总点击量'
          value={data.totalClicks.value}
          changeText={data.totalClicks.changeText}
          icon='trend'
        />
        <MetricCard
          title='独立访客'
          value={data.uniqueVisitors.value}
          changeText={data.uniqueVisitors.changeText}
          icon='user'
        />
        <MetricCard
          title='跳出率'
          value={`${data.bounceRate.value}%`}
          changeText={data.bounceRate.changeText}
          icon='pulse'
        />
        <MetricCard
          title='平均会话'
          value={formatDuration(data.avgSession.value)}
          changeText={data.avgSession.changeText}
          icon='clock'
        />
      </div>
      <div className='grid grid-cols-1 gap-4 lg:grid-cols-7'>
        <Card className='col-span-1 lg:col-span-4'>
          <CardHeader>
            <CardTitle>来源渠道</CardTitle>
            <CardDescription>带来访问量的主要来源</CardDescription>
          </CardHeader>
          <CardContent>
            <SimpleBarList
              items={data.referrers}
              barClass='bg-primary'
              valueFormatter={(n) => `${n}`}
              emptyText='暂无来源数据。'
            />
          </CardContent>
        </Card>
        <Card className='col-span-1 lg:col-span-3'>
          <CardHeader>
            <CardTitle>设备分布</CardTitle>
            <CardDescription>用户访问应用的设备类型</CardDescription>
          </CardHeader>
          <CardContent>
            <SimpleBarList
              items={data.devices}
              barClass='bg-muted-foreground'
              valueFormatter={(n) => `${n}%`}
              emptyText='暂无设备数据。'
            />
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

function MetricCard({
  title,
  value,
  changeText,
  icon,
}: {
  title: string
  value: number | string
  changeText: string
  icon: 'trend' | 'user' | 'pulse' | 'clock'
}) {
  return (
    <Card>
      <CardHeader className='flex flex-row items-center justify-between space-y-0 pb-2'>
        <CardTitle className='text-sm font-medium'>{title}</CardTitle>
        <MetricIcon icon={icon} />
      </CardHeader>
      <CardContent>
        <div className='text-2xl font-bold'>{value}</div>
        <p className='text-xs text-muted-foreground'>{changeText}</p>
      </CardContent>
    </Card>
  )
}

function MetricIcon({ icon }: { icon: 'trend' | 'user' | 'pulse' | 'clock' }) {
  if (icon === 'user') {
    return (
      <svg
        xmlns='http://www.w3.org/2000/svg'
        viewBox='0 0 24 24'
        fill='none'
        stroke='currentColor'
        strokeLinecap='round'
        strokeLinejoin='round'
        strokeWidth='2'
        className='h-4 w-4 text-muted-foreground'
      >
        <circle cx='12' cy='7' r='4' />
        <path d='M6 21v-2a6 6 0 0 1 12 0v2' />
      </svg>
    )
  }

  if (icon === 'pulse') {
    return (
      <svg
        xmlns='http://www.w3.org/2000/svg'
        viewBox='0 0 24 24'
        fill='none'
        stroke='currentColor'
        strokeLinecap='round'
        strokeLinejoin='round'
        strokeWidth='2'
        className='h-4 w-4 text-muted-foreground'
      >
        <path d='M3 12h6l3 6 3-6h6' />
      </svg>
    )
  }

  if (icon === 'clock') {
    return (
      <svg
        xmlns='http://www.w3.org/2000/svg'
        viewBox='0 0 24 24'
        fill='none'
        stroke='currentColor'
        strokeLinecap='round'
        strokeLinejoin='round'
        strokeWidth='2'
        className='h-4 w-4 text-muted-foreground'
      >
        <circle cx='12' cy='12' r='10' />
        <path d='M12 6v6l4 2' />
      </svg>
    )
  }

  return (
    <svg
      xmlns='http://www.w3.org/2000/svg'
      viewBox='0 0 24 24'
      fill='none'
      stroke='currentColor'
      strokeLinecap='round'
      strokeLinejoin='round'
      strokeWidth='2'
      className='h-4 w-4 text-muted-foreground'
    >
      <path d='M3 3v18h18' />
      <path d='M7 15l4-4 4 4 4-6' />
    </svg>
  )
}

function SimpleBarList({
  items,
  valueFormatter,
  barClass,
  emptyText,
}: {
  items: { name: string; value: number }[]
  valueFormatter: (n: number) => string
  barClass: string
  emptyText: string
}) {
  if (!items.length) {
    return (
      <div className='py-10 text-center text-sm text-muted-foreground'>
        {emptyText}
      </div>
    )
  }

  const max = Math.max(...items.map((i) => i.value), 1)
  return (
    <ul className='space-y-3'>
      {items.map((i) => {
        const width = `${Math.round((i.value / max) * 100)}%`
        return (
          <li key={i.name} className='flex items-center justify-between gap-3'>
            <div className='min-w-0 flex-1'>
              <div className='mb-1 truncate text-xs text-muted-foreground'>
                {i.name}
              </div>
              <div className='h-2.5 w-full rounded-full bg-muted'>
                <div
                  className={`h-2.5 rounded-full ${barClass}`}
                  style={{ width }}
                />
              </div>
            </div>
            <div className='ps-2 text-xs font-medium tabular-nums'>
              {valueFormatter(i.value)}
            </div>
          </li>
        )
      })}
    </ul>
  )
}
