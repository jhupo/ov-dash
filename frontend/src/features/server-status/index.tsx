import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import {
  Cpu,
  HardDrive,
  Info,
  MemoryStick,
  RefreshCw,
} from 'lucide-react'
import {
  listServerMetrics,
  listServerConnections,
  touchServerMonitor,
  type ServerConnection,
  type ServerMetric,
} from '@/services/server-connections'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ConfigDrawer } from '@/components/config-drawer'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'

type MetricRow = {
  label: string
  value: string
  percent: number
  tone: 'green' | 'yellow' | 'red' | 'muted'
}

type ChartPoint = {
  time: string
  cpu: number
  latency: number
  memory: number
  disk: number
  network_rx: number
  network_tx: number
  tcp: number
  udp: number
  processes: number
}

export function ServerStatus() {
  const [activeGroup, setActiveGroup] = useState('全部')
  const [detailId, setDetailId] = useState<string | null>(null)
  const [statsId, setStatsId] = useState<string | null>(null)
  const servers = useQuery({
    queryKey: ['server-connections'],
    queryFn: listServerConnections,
    refetchInterval: 1_000,
  })

  useEffect(() => {
    void touchServerMonitor()
    const timer = window.setInterval(() => {
      void touchServerMonitor()
    }, 10_000)
    return () => window.clearInterval(timer)
  }, [])

  const items = servers.data ?? []
  const detail = useMemo(
    () => items.find((item) => item.id === detailId) ?? null,
    [detailId, items]
  )
  const stats = useMemo(
    () => items.find((item) => item.id === statsId) ?? null,
    [statsId, items]
  )
  const groups = useMemo(() => {
    const names = new Set(items.map((item) => item.group_name || '默认'))
    return ['全部', ...Array.from(names)]
  }, [items])
  const filtered = useMemo(() => {
    if (activeGroup === '全部') return items
    return items.filter((item) => (item.group_name || '默认') === activeGroup)
  }, [activeGroup, items])
  const regions = useMemo(
    () => new Set(items.map((item) => item.region.trim()).filter(Boolean)).size,
    [items]
  )
  const online = items.filter((item) => item.collect_status === 'ok').length
  const traffic = items.reduce(
    (sum, item) => {
      sum.rx += item.metric?.network_rx_bytes ?? 0
      sum.tx += item.metric?.network_tx_bytes ?? 0
      sum.rxRate += item.metric?.network_rx_rate_bps ?? 0
      sum.txRate += item.metric?.network_tx_rate_bps ?? 0
      return sum
    },
    { rx: 0, tx: 0, rxRate: 0, txRate: 0 }
  )

  return (
    <>
      <Header>
        <Search className='me-auto' />
        <ThemeSwitch />
        <ConfigDrawer />
        <ProfileDropdown />
      </Header>

      <Main>
        <div className='mx-auto flex w-full max-w-[1540px] flex-col gap-4'>
          <SummaryBar
            online={online}
            total={items.length}
            regions={regions}
            traffic={traffic}
            isFetching={servers.isFetching}
            onRefresh={() => servers.refetch()}
          />

          <div className='flex min-h-10 items-center gap-2 rounded-md border bg-card/70 px-3'>
            <span className='text-sm text-muted-foreground'>分组</span>
            <div className='flex flex-wrap gap-1'>
              {groups.map((group) => (
                <Button
                  key={group}
                  type='button'
                  size='sm'
                  variant={activeGroup === group ? 'secondary' : 'ghost'}
                  className='h-7 px-3'
                  onClick={() => setActiveGroup(group)}
                >
                  {group}
                </Button>
              ))}
            </div>
          </div>

          {filtered.length ? (
            <div className='flex flex-wrap gap-4'>
              {filtered.map((server) => (
                <ServerCard
                  key={server.id}
                  server={server}
                  onStats={() => setStatsId(server.id)}
                  onDetail={() => setDetailId(server.id)}
                />
              ))}
            </div>
          ) : (
            <div className='flex min-h-[360px] flex-col items-center justify-center gap-3 rounded-md border border-dashed bg-card/50'>
              <p className='text-sm text-muted-foreground'>
                还没有服务器配置
              </p>
              <Button asChild>
                <Link to='/settings/servers'>添加服务器</Link>
              </Button>
            </div>
          )}
        </div>
      </Main>

      <ServerDetailDialog
        server={detail}
        onOpenChange={() => setDetailId(null)}
      />
      <ServerStatsDialog server={stats} onOpenChange={() => setStatsId(null)} />
    </>
  )
}

function SummaryBar({
  online,
  total,
  regions,
  traffic,
  isFetching,
  onRefresh,
}: {
  online: number
  total: number
  regions: number
  traffic: { rx: number; tx: number; rxRate: number; txRate: number }
  isFetching: boolean
  onRefresh: () => void
}) {
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), 1000)
    return () => window.clearInterval(timer)
  }, [])
  const time = now.toLocaleTimeString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })

  return (
    <div className='grid min-h-20 grid-cols-2 items-center gap-3 rounded-md border bg-card/80 px-4 py-3 shadow-sm md:grid-cols-5 xl:grid-cols-[1fr_1fr_1fr_1fr_1fr_auto]'>
      <SummaryItem label='当前时间' value={time} />
      <SummaryItem label='当前在线' value={`${online} / ${total}`} />
      <SummaryItem label='点亮地区' value={String(regions)} />
      <SummaryItem
        label='流量概览'
        value={`↑ ${formatBytes(traffic.tx)}`}
        subValue={`↓ ${formatBytes(traffic.rx)}`}
      />
      <SummaryItem
        label='网络速率'
        value={`↑ ${formatRate(traffic.txRate)}`}
        subValue={`↓ ${formatRate(traffic.rxRate)}`}
      />
      <Button
        type='button'
        variant='ghost'
        size='icon'
        className='ms-auto'
        onClick={onRefresh}
        aria-label='刷新服务器状态'
      >
        <RefreshCw className={isFetching ? 'animate-spin' : undefined} />
      </Button>
    </div>
  )
}

function SummaryItem({
  label,
  value,
  subValue,
}: {
  label: string
  value: string
  subValue?: string
}) {
  return (
    <div className='min-w-0 text-center text-foreground'>
      <div className='text-xs text-muted-foreground'>{label}</div>
      <div className='truncate text-sm font-semibold tabular-nums'>{value}</div>
      {subValue && (
        <div className='truncate text-sm font-semibold tabular-nums'>
          {subValue}
        </div>
      )}
    </div>
  )
}

function ServerCard({
  server,
  onStats,
  onDetail,
}: {
  server: ServerConnection
  onStats: () => void
  onDetail: () => void
}) {
  const metric = server.metric
  const rows = buildRows(metric)

  return (
    <article className='min-h-[350px] w-full max-w-[294px] rounded-lg border bg-card/80 p-4 text-card-foreground shadow-md backdrop-blur'>
      <div className='flex items-start justify-between gap-3 border-b pb-3'>
        <div className='min-w-0 flex-1'>
          <div className='flex min-w-0 items-center gap-2'>
            <RegionMark region={server.region} />
            <SystemMark metric={metric} />
            <button
              type='button'
              className='min-w-0 truncate text-left text-base font-bold underline-offset-4 hover:underline'
              onClick={onStats}
            >
              {server.name}
            </button>
          </div>
          {metric ? (
            <div className='mt-3 grid grid-cols-3 items-start gap-2 text-xs text-muted-foreground'>
              <IconValue icon={<Cpu />} value={cpuCoreText(metric)} />
              <IconValue icon={<MemoryStick />} value={memoryText(metric)} />
              <IconValue icon={<HardDrive />} value={diskText(metric)} />
            </div>
          ) : (
            <div className='mt-3 text-center text-xs text-muted-foreground'>
              待采集
            </div>
          )}
        </div>
        <Button
          type='button'
          size='icon'
          variant='ghost'
          className='size-8 shrink-0'
          onClick={onDetail}
          aria-label='查看服务器详情'
        >
          <Info className='size-5' />
        </Button>
      </div>

      <div className='mt-3 space-y-3'>
        {rows.map((row) => (
          <ProgressRow key={row.label} row={row} />
        ))}
      </div>

      <div className='mt-4 border-t pt-3 text-xs'>
        <InfoLine
          label='网络'
          value={
            metric
              ? `↑ ${formatRate(metric.network_tx_rate_bps)} / ↓ ${formatRate(metric.network_rx_rate_bps)}`
              : '待采集'
          }
        />
      </div>

      <div className='mt-2 grid gap-2 text-xs'>
        <InfoLine
          label='流量'
          value={
            metric
              ? `↑ ${formatBytes(metric.network_tx_bytes)} / ↓ ${formatBytes(metric.network_rx_bytes)}`
              : '待采集'
          }
        />
        <InfoLine
          label='负载'
          value={
            metric
              ? `${metric.load1.toFixed(2)} | ${metric.load5.toFixed(2)} | ${metric.load15.toFixed(2)}`
              : '待采集'
          }
        />
      </div>

      <div className='mt-4 grid grid-cols-[1fr_auto_1fr] items-center gap-3 text-xs text-muted-foreground'>
        <span className='truncate'>到期: {formatDate(server.expires_at)}</span>
        <span className='h-4 w-px bg-border' />
        <span className='truncate text-right'>
          {server.last_collected_at
            ? formatTime(server.last_collected_at)
            : cardStatusText(server)}
        </span>
      </div>
    </article>
  )
}

function ServerDetailDialog({
  server,
  onOpenChange,
}: {
  server: ServerConnection | null
  onOpenChange: () => void
}) {
  const metric = server?.metric
  return (
    <Dialog open={!!server} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-4xl'>
        <DialogHeader>
          <DialogTitle>{server?.name ?? '服务器详情'}</DialogTitle>
        </DialogHeader>
        {server && (
          <div className='grid gap-x-10 gap-y-5 text-sm md:grid-cols-3'>
            <DetailItem label='CPU' value={metric?.cpu_model || '待采集'} />
            <DetailItem label='架构' value={metric?.architecture || '待采集'} />
            <DetailItem
              label='虚拟化'
              value={metric?.virtualization || '待采集'}
            />
            <DetailItem label='GPU' value={metric?.gpu_model || 'None'} />
            <DetailItem label='操作系统' value={metric?.os_name || '待采集'} />
            <DetailItem
              label='内存'
              value={
                metric
                  ? `${formatBytes(metric.memory_used_bytes)} / ${formatBytes(metric.memory_total_bytes)}`
                  : '待采集'
              }
            />
            <DetailItem
              label='硬盘'
              value={
                metric
                  ? `${formatBytes(metric.disk_used_bytes)} / ${formatBytes(metric.disk_total_bytes)}`
                  : '待采集'
              }
            />
            <DetailItem
              label='实时网络'
              value={
                metric
                  ? `↑ ${formatRate(metric.network_tx_rate_bps)} / ↓ ${formatRate(metric.network_rx_rate_bps)}`
                  : '待采集'
              }
            />
            <DetailItem
              label='总流量'
              value={
                metric
                  ? `↑ ${formatBytes(metric.network_tx_bytes)} / ↓ ${formatBytes(metric.network_rx_bytes)}`
                  : '待采集'
              }
            />
            <DetailItem
              label='运行时间'
              value={metric ? formatDuration(metric.uptime_seconds) : '待采集'}
            />
            <DetailItem
              label='最后上报'
              value={
                server.last_collected_at
                  ? formatDateTime(server.last_collected_at)
                  : '待采集'
              }
            />
            <DetailItem label='到期时间' value={formatDate(server.expires_at)} />
            <DetailItem label='连接' value={server.connection_hint} />
            <DetailItem label='采集状态' value={statusText(server)} />
            <DetailItem
              label='错误'
              value={server.collect_error || '无'}
              className='md:col-span-3'
            />
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

function ServerStatsDialog({
  server,
  onOpenChange,
}: {
  server: ServerConnection | null
  onOpenChange: () => void
}) {
  const [range, setRange] = useState('1h')
  const [view, setView] = useState<'load' | 'latency'>('load')
  const metrics = useQuery({
    queryKey: ['server-metrics', server?.id, range],
    queryFn: () => listServerMetrics(server!.id, range),
    enabled: !!server,
    refetchInterval: range === '1h' ? 1_000 : 30_000,
  })
  const points = useMemo(
    () => (metrics.data ?? []).map(toChartPoint),
    [metrics.data]
  )
  const latest = metrics.data?.length
    ? metrics.data[metrics.data.length - 1]
    : (server?.metric ?? null)

  return (
    <Dialog open={!!server} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[92vh] overflow-y-auto p-0 sm:max-w-[1160px]'>
        <DialogHeader className='sr-only'>
          <DialogTitle>{server?.name ?? '负载统计'}</DialogTitle>
        </DialogHeader>
        <div className='relative space-y-4 p-6'>
          <div className='flex items-center justify-center'>
            <div className='inline-flex rounded-md border bg-muted/60 p-1'>
              {[
                ['负载', 'load'],
                ['延迟', 'latency'],
              ].map(([label, value]) => (
                <Button
                  key={value}
                  type='button'
                  size='sm'
                  variant={view === value ? 'secondary' : 'ghost'}
                  className='h-8 px-5'
                  onClick={() => setView(value as 'load' | 'latency')}
                >
                  {label}
                </Button>
              ))}
            </div>
          </div>

          <div className='flex items-center justify-center'>
            <div className='inline-flex rounded-md border bg-muted/60 p-1'>
            {[
              ['实时', '1h'],
              ['4小时', '4h'],
              ['1天', '1d'],
              ['7天', '7d'],
              ['30天', '30d'],
            ].map(([label, value]) => (
              <Button
                key={value}
                type='button'
                size='sm'
                variant={range === value ? 'secondary' : 'ghost'}
                className='h-8 px-4'
                onClick={() => setRange(value)}
              >
                {label}
              </Button>
            ))}
            </div>
          </div>

          {points.length && view === 'load' ? (
            <div className='grid gap-4 lg:grid-cols-3'>
              <StatsChart
                title='CPU'
                value={latest ? `${latest.cpu_percent.toFixed(2)}%` : '待采集'}
                data={points}
                lines={[{ key: 'cpu', name: 'CPU', color: '#f87171' }]}
                unit='%'
              />
              <StatsChart
                title='内存'
                value={
                  latest
                    ? `${formatBytes(latest.memory_used_bytes)} / ${formatBytes(latest.memory_total_bytes)}`
                    : '待采集'
                }
                data={points}
                lines={[{ key: 'memory', name: '内存', color: '#f59e0b' }]}
                unit='%'
              />
              <StatsChart
                title='磁盘'
                value={
                  latest
                    ? `${formatBytes(latest.disk_used_bytes)} / ${formatBytes(latest.disk_total_bytes)}`
                    : '待采集'
                }
                data={points}
                lines={[{ key: 'disk', name: '磁盘', color: '#fb7185' }]}
                unit='%'
              />
              <StatsChart
                title='网络'
                value={
                  latest
                    ? `↑ ${formatRate(latest.network_tx_rate_bps)}  ↓ ${formatRate(latest.network_rx_rate_bps)}`
                    : '待采集'
                }
                data={points}
                lines={[
                  { key: 'network_tx', name: '上传', color: '#fb7185' },
                  { key: 'network_rx', name: '下载', color: '#38bdf8' },
                ]}
              />
              <StatsChart
                title='连接数'
                value={
                  latest
                    ? `TCP: ${latest.tcp_connections}  UDP: ${latest.udp_connections}`
                    : '待采集'
                }
                data={points}
                lines={[
                  { key: 'tcp', name: 'TCP', color: '#f87171' },
                  { key: 'udp', name: 'UDP', color: '#facc15' },
                ]}
              />
              <StatsChart
                title='进程数'
                value={latest ? String(latest.process_count) : '待采集'}
                data={points}
                lines={[
                  { key: 'processes', name: '进程', color: '#f87171' },
                ]}
              />
            </div>
          ) : points.length ? (
            <div className='grid gap-4 lg:grid-cols-3'>
              <StatsChart
                title='延迟'
                value={
                  latest?.latency_ms
                    ? `${latest.latency_ms.toFixed(0)} ms`
                    : '待采集'
                }
                data={points}
                lines={[{ key: 'latency', name: '延迟', color: '#38bdf8' }]}
                unit='ms'
              />
              <StatsChart
                title='网络'
                value={
                  latest
                    ? `↑ ${formatRate(latest.network_tx_rate_bps)}  ↓ ${formatRate(latest.network_rx_rate_bps)}`
                    : '待采集'
                }
                data={points}
                lines={[
                  { key: 'network_tx', name: '上传', color: '#fb7185' },
                  { key: 'network_rx', name: '下载', color: '#38bdf8' },
                ]}
              />
              <StatsChart
                title='连接数'
                value={
                  latest
                    ? `TCP: ${latest.tcp_connections}  UDP: ${latest.udp_connections}`
                    : '待采集'
                }
                data={points}
                lines={[
                  { key: 'tcp', name: 'TCP', color: '#f87171' },
                  { key: 'udp', name: 'UDP', color: '#facc15' },
                ]}
              />
            </div>
          ) : (
            <div className='flex min-h-[470px] items-center justify-center rounded-md border border-dashed text-sm text-muted-foreground'>
              {metrics.isFetching ? '正在读取负载统计...' : '暂无采集历史'}
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}

function StatsChart({
  title,
  value,
  data,
  lines,
  unit = '',
}: {
  title: string
  value: string
  data: ChartPoint[]
  lines: { key: keyof ChartPoint; name: string; color: string }[]
  unit?: string
}) {
  return (
    <div className='h-56 rounded-md border bg-card/80 p-4'>
      <div className='mb-3 flex items-start justify-between gap-3'>
        <div className='text-sm font-medium'>{title}</div>
        <div className='max-w-48 text-right text-sm font-semibold tabular-nums'>
          {value}
        </div>
      </div>
      <ResponsiveContainer width='100%' height='78%'>
        <AreaChart data={data}>
          <CartesianGrid strokeDasharray='3 3' className='stroke-border' />
          <XAxis
            dataKey='time'
            tickLine={false}
            axisLine={false}
            tick={{ fontSize: 11 }}
          />
          <YAxis hide domain={unit === '%' ? [0, 100] : ['auto', 'auto']} />
          <Tooltip
            formatter={(value, name) => [
              `${Number(value).toFixed(unit === '%' ? 1 : 0)}${unit ? ` ${unit}` : ''}`,
              name,
            ]}
            contentStyle={{
              background: 'hsl(var(--card))',
              border: '1px solid hsl(var(--border))',
              borderRadius: 6,
            }}
          />
          {lines.map((line) => (
            <Area
              key={String(line.key)}
              type='monotone'
              dataKey={line.key}
              name={line.name}
              stroke={line.color}
              fill={line.color}
              fillOpacity={0.18}
              strokeWidth={1.5}
            />
          ))}
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}

function DetailItem({
  label,
  value,
  className,
}: {
  label: string
  value: string
  className?: string
}) {
  return (
    <div className={className}>
      <div className='text-muted-foreground'>{label}</div>
      <div className='mt-1 break-words font-medium'>{value}</div>
    </div>
  )
}

function IconValue({
  icon,
  value,
}: {
  icon: ReactNode
  value: string
}) {
  return (
    <span className='flex min-w-0 flex-col items-center justify-start gap-1 text-center'>
      <span className='[&_svg]:size-3.5 [&_svg]:text-primary'>{icon}</span>
      <span className='w-full text-[11px] leading-tight break-words tabular-nums'>
        {value}
      </span>
    </span>
  )
}

function ProgressRow({ row }: { row: MetricRow }) {
  const toneClass =
    row.tone === 'green'
      ? 'bg-emerald-500'
      : row.tone === 'yellow'
        ? 'bg-amber-400'
        : row.tone === 'red'
          ? 'bg-red-500'
          : 'bg-muted-foreground/25'

  return (
    <div className='grid grid-cols-[3rem_minmax(0,1fr)_3rem] items-center gap-3 text-sm'>
      <span>{row.label}</span>
      <div className='h-3 overflow-hidden rounded-full bg-muted'>
        <div
          className={`h-full rounded-full ${toneClass}`}
          style={{
            width: `${Math.max(row.percent > 0 ? 2 : 0, Math.min(100, row.percent))}%`,
          }}
        />
      </div>
      <span className='text-right text-xs'>{row.value}</span>
    </div>
  )
}

function InfoLine({ label, value }: { label: string; value: string }) {
  return (
    <div className='grid grid-cols-[3rem_minmax(0,1fr)] gap-3'>
      <span>{label}</span>
      <span className='truncate text-right text-foreground'>{value}</span>
    </div>
  )
}

function RegionMark({ region }: { region: string }) {
  const normalized = region.trim().toLowerCase()
  if (!normalized || normalized === 'utc' || normalized === 'etc/utc') {
    return null
  }
  const flag = normalized.includes('hk')
    ? '🇭🇰'
    : normalized.includes('us') || normalized.includes('usa')
      ? '🇺🇸'
      : normalized.includes('jp')
        ? '🇯🇵'
        : normalized.includes('sg') || normalized.includes('singapore')
          ? '🇸🇬'
          : normalized.includes('kr') || normalized.includes('korea')
            ? '🇰🇷'
            : normalized.includes('de') || normalized.includes('germany')
              ? '🇩🇪'
              : normalized.includes('gb') ||
                  normalized.includes('uk') ||
                  normalized.includes('london')
                ? '🇬🇧'
                : ''
  if (flag) return <span className='text-lg leading-none'>{flag}</span>
  if (!region.trim()) return null
  return <span className='text-xs font-semibold text-primary'>{region}</span>
}

function SystemMark({ metric }: { metric: ServerMetric | null }) {
  const os = metric?.os_name.toLowerCase() ?? ''
  const label = os.includes('ubuntu')
    ? 'Ubuntu'
    : os.includes('debian')
      ? 'Debian'
      : os.includes('centos') || os.includes('rocky') || os.includes('alma')
        ? 'EL'
        : os.includes('alpine')
          ? 'Alpine'
          : ''

  if (!label) {
    return null
  }

  return (
    <span className='flex h-5 min-w-5 items-center justify-center rounded-sm bg-orange-500 px-1 text-[10px] font-bold text-white'>
      {label.slice(0, 2)}
    </span>
  )
}

function buildRows(metric: ServerMetric | null): MetricRow[] {
  if (!metric) {
    return ['CPU', '内存', 'SWAP', '硬盘'].map((label) => ({
      label,
      value: '--',
      percent: 0,
      tone: 'muted',
    }))
  }
  const memory = percent(metric.memory_used_bytes, metric.memory_total_bytes)
  const swap = percent(metric.swap_used_bytes, metric.swap_total_bytes)
  const disk = percent(metric.disk_used_bytes, metric.disk_total_bytes)
  return [
    {
      label: 'CPU',
      value: formatPercent(metric.cpu_percent),
      percent: metric.cpu_percent,
      tone: tone(metric.cpu_percent),
    },
    {
      label: '内存',
      value: `${memory.toFixed(0)}%`,
      percent: memory,
      tone: tone(memory),
    },
    {
      label: 'SWAP',
      value: metric.swap_total_bytes ? `${swap.toFixed(0)}%` : 'OFF',
      percent: swap,
      tone: metric.swap_total_bytes ? tone(swap) : 'muted',
    },
    {
      label: '硬盘',
      value: `${disk.toFixed(0)}%`,
      percent: disk,
      tone: tone(disk),
    },
  ]
}

function percent(used: number, total: number) {
  if (!total) return 0
  return (used / total) * 100
}

function tone(value: number): MetricRow['tone'] {
  if (value >= 90) return 'red'
  if (value >= 60) return 'yellow'
  return 'green'
}

function formatPercent(value: number) {
  if (value > 0 && value < 10) return `${value.toFixed(1)}%`
  return `${value.toFixed(0)}%`
}

function memoryText(metric: ServerMetric | null) {
  if (!metric) return '待采集'
  return formatBytes(metric.memory_total_bytes)
}

function cpuCoreText(metric: ServerMetric | null) {
  if (!metric?.cpu_cores) return 'CPU'
  return `${metric.cpu_cores} Cores`
}

function diskText(metric: ServerMetric | null) {
  if (!metric) return '待采集'
  return formatBytes(metric.disk_total_bytes)
}

function toChartPoint(metric: ServerMetric): ChartPoint {
  return {
    time: new Date(metric.collected_at).toLocaleTimeString('zh-CN', {
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    }),
    cpu: metric.cpu_percent,
    latency: metric.latency_ms,
    memory: percent(metric.memory_used_bytes, metric.memory_total_bytes),
    disk: percent(metric.disk_used_bytes, metric.disk_total_bytes),
    network_rx: metric.network_rx_rate_bps,
    network_tx: metric.network_tx_rate_bps,
    tcp: metric.tcp_connections,
    udp: metric.udp_connections,
    processes: metric.process_count,
  }
}

function formatBytes(value: number) {
  if (!value) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = value
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index += 1
  }
  return `${size >= 10 || index === 0 ? size.toFixed(0) : size.toFixed(2)} ${units[index]}`
}

function formatRate(value: number) {
  return `${formatBytes(value)}/s`
}

function formatDate(value: string | null) {
  if (!value) return '未设置'
  return new Date(value).toLocaleDateString('zh-CN')
}

function formatDateTime(value: string) {
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

function formatTime(value: string) {
  return new Date(value).toLocaleTimeString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })
}

function formatDuration(seconds: number) {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  if (days > 0) return `${days}天${hours}小时`
  return `${hours}小时`
}

function statusText(server: ServerConnection) {
  if (server.collect_status === 'ok') return '在线'
  if (server.collect_status === 'collecting') return '采集中'
  if (server.collect_status === 'error') return '异常'
  return '等待采集'
}

function cardStatusText(server: ServerConnection) {
  if (server.collect_status === 'ok') return '在线'
  if (server.collect_status === 'collecting') return '采集中'
  return '待采集'
}
