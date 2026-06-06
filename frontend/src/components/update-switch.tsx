import axios from 'axios'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, CloudUpload, RefreshCw, Rocket, RotateCw } from 'lucide-react'
import { toast } from 'sonner'
import {
  applyUpdate,
  checkUpdate,
  getUpdateStatus,
  restartUpdate,
  type UpdateRun,
  type UpdateStatus,
} from '@/services/updates'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

export function UpdateSwitch() {
  const queryClient = useQueryClient()
  const status = useQuery({
    queryKey: ['updates'],
    queryFn: getUpdateStatus,
    refetchInterval: (query) => {
      const update = query.state.data?.update
      return update?.status === 'running' || update?.status === 'restarting'
        ? 2_000
        : false
    },
  })
  const value = status.data?.status
  const update = status.data?.update

  const checkMutation = useMutation({
    mutationFn: checkUpdate,
    onSuccess: async (result) => {
      await queryClient.invalidateQueries({ queryKey: ['updates'] })
      toast[result.hasUpdate ? 'success' : 'info'](result.message)
    },
    onError: (error) => toast.error(errorMessage(error, '检查更新失败')),
  })

  const applyMutation = useMutation({
    mutationFn: applyUpdate,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['updates'] })
      toast.success('开始准备更新')
    },
    onError: (error) => toast.error(errorMessage(error, '启动更新失败')),
  })

  const restartMutation = useMutation({
    mutationFn: restartUpdate,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['updates'] })
      toast.success('服务正在重启')
    },
    onError: (error) => toast.error(errorMessage(error, '重启服务失败')),
  })

  const busy =
    status.isFetching ||
    checkMutation.isPending ||
    applyMutation.isPending ||
    restartMutation.isPending
  const canApply = Boolean(value?.hasUpdate && !busy && !isActiveUpdate(update))
  const canRestart = update?.status === 'ready' && !busy

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger asChild>
        <Button variant='ghost' size='icon' className='rounded-full'>
          <CloudUpload className='size-[1.2rem]' />
          <span className='sr-only'>在线更新</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='w-[380px] p-0'>
        <div className='space-y-4 p-4'>
          <div className='flex items-start justify-between gap-4'>
            <div className='min-w-0'>
              <div className='text-sm font-semibold'>在线更新</div>
              <div className='mt-3 text-xs text-muted-foreground'>当前版本</div>
              <div className='mt-1 truncate text-sm font-semibold tabular-nums'>
                {versionText(value)}
              </div>
            </div>
            <div className='flex shrink-0 flex-col items-end gap-3'>
              <StatusBadge status={value} update={update} />
              <Button
                type='button'
                size='sm'
                variant='outline'
                disabled={busy}
                onClick={() => checkMutation.mutate()}
              >
                <RefreshCw className={busy ? 'animate-spin' : undefined} />
                刷新
              </Button>
            </div>
          </div>

          {(value?.hasUpdate || update) && (
            <>
              <DropdownMenuSeparator />
              <div className='space-y-3'>
                {value?.hasUpdate && (
                  <div className='rounded-md border bg-muted/30 p-3'>
                    <div className='flex items-center justify-between gap-3'>
                      <span className='text-xs text-muted-foreground'>发现新版本</span>
                      <span className='font-mono text-sm font-semibold'>
                        {value.latestVersion}
                      </span>
                    </div>
                    <div className='mt-2 text-xs text-muted-foreground'>
                      检查时间 {formatDateTime(value.checkedAt)}
                    </div>
                  </div>
                )}

                {update && <UpdateProgress update={update} />}

                <div className='grid grid-cols-2 gap-2'>
                  <Button
                    type='button'
                    variant={canRestart ? 'outline' : 'default'}
                    disabled={!canApply}
                    onClick={() => applyMutation.mutate()}
                  >
                    <Rocket />
                    更新
                  </Button>
                  <Button
                    type='button'
                    disabled={!canRestart}
                    onClick={() => restartMutation.mutate()}
                  >
                    <RotateCw />
                    立即重启
                  </Button>
                </div>
              </div>
            </>
          )}
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function StatusBadge({
  status,
  update,
}: {
  status?: UpdateStatus
  update?: UpdateRun | null
}) {
  if (update?.status === 'running') return <Badge variant='secondary'>准备中</Badge>
  if (update?.status === 'ready') return <Badge>待重启</Badge>
  if (update?.status === 'restarting') return <Badge variant='secondary'>重启中</Badge>
  if (status?.hasUpdate) return <Badge>有更新</Badge>
  return <Badge variant='outline'>当前版本</Badge>
}

function UpdateProgress({ update }: { update: UpdateRun }) {
  const progress = Math.max(0, Math.min(100, update.progress || 0))
  const isDone = update.status === 'ready' || update.status === 'success'
  const isError = update.status === 'error'

  return (
    <div className='space-y-2 rounded-md border p-3'>
      <div className='flex items-center justify-between gap-3 text-sm'>
        <span className='font-medium'>{update.message}</span>
        {isDone ? (
          <CheckCircle2 className='size-4 text-emerald-500' />
        ) : (
          <span className='font-mono text-xs text-muted-foreground'>{progress}%</span>
        )}
      </div>
      <div className='h-2 overflow-hidden rounded-full bg-muted'>
        <div
          className={
            isError
              ? 'h-full bg-destructive transition-all'
              : 'h-full bg-primary transition-all'
          }
          style={{ width: `${isError ? 100 : progress}%` }}
        />
      </div>
      {update.status === 'ready' && (
        <div className='text-xs text-muted-foreground'>
          更新已准备完成，点击“立即重启”完成切换。
        </div>
      )}
      {isError && (
        <div className='max-h-20 overflow-auto text-xs text-destructive'>
          {update.message}
        </div>
      )}
    </div>
  )
}

function isActiveUpdate(update?: UpdateRun | null) {
  return update?.status === 'running' || update?.status === 'restarting'
}

function versionText(status?: UpdateStatus) {
  if (!status) return '读取中'
  if (status.currentVersion && status.currentVersion !== 'local') {
    return status.currentVersion
  }
  return status.currentCommit ? `local (${status.currentCommit})` : 'local'
}

function formatDateTime(value?: string) {
  if (!value) return '未检查'
  return new Date(value).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function errorMessage(error: unknown, fallback: string) {
  if (axios.isAxiosError(error)) {
    const data = error.response?.data
    if (data && typeof data === 'object' && 'error' in data) {
      if ('message' in data && data.message) {
        return String(data.message)
      }
      return String(data.error)
    }
  }
  return fallback
}
