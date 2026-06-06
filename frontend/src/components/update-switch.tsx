import axios from 'axios'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CheckCircle2,
  CloudUpload,
  RefreshCw,
  RotateCw,
  UploadCloud,
} from 'lucide-react'
import { toast } from 'sonner'
import {
  applyUpdate,
  checkUpdate,
  getUpdateStatus,
  restartUpdate,
  type UpdateRun,
  type UpdateStatus,
} from '@/services/updates'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
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
  const showUpdatePanel =
    Boolean(value?.hasUpdate) || Boolean(update && update.status !== 'success')

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger asChild>
        <Button
          variant='ghost'
          size='icon'
          className='relative rounded-full'
          aria-label='在线更新'
        >
          <CloudUpload className='size-[1.2rem]' />
          {(value?.hasUpdate || isActiveUpdate(update) || canRestart) && (
            <span className='absolute top-1.5 right-1.5 size-2 rounded-full bg-primary ring-2 ring-background' />
          )}
          <span className='sr-only'>在线更新</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='w-[288px] p-0'>
        <div className='space-y-2.5 p-2.5'>
          <div className='grid grid-cols-[minmax(0,1fr)_2rem] items-center gap-2'>
            <div className='grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-2 rounded-md border bg-muted/20 px-2.5 py-2'>
              <span className='text-xs text-muted-foreground'>当前版本</span>
              <span className='truncate text-right font-mono text-sm font-semibold tracking-normal'>
                {versionText(value)}
              </span>
            </div>
            <Button
              type='button'
              size='icon'
              variant='ghost'
              className='size-8'
              disabled={busy}
              onClick={() => checkMutation.mutate()}
              aria-label='刷新版本'
            >
              <RefreshCw className={busy ? 'animate-spin' : undefined} />
            </Button>
          </div>

          {showUpdatePanel ? (
            <div className='space-y-2.5 rounded-md border bg-muted/25 p-2.5'>
              {value?.hasUpdate && (
                <div className='grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3'>
                  <div className='grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-2'>
                    <span className='text-xs text-muted-foreground'>更新版本</span>
                    <span className='truncate text-right font-mono text-sm font-semibold'>
                      {value.latestVersion}
                    </span>
                  </div>
                  {!isActiveUpdate(update) && update?.status !== 'ready' && (
                    <Button
                      type='button'
                      size='sm'
                      disabled={!canApply}
                      onClick={() => applyMutation.mutate()}
                    >
                      <UploadCloud />
                      更新
                    </Button>
                  )}
                </div>
              )}

              {update && <UpdateProgress update={update} />}

              {canRestart && (
                <Button
                  type='button'
                  className='w-full'
                  disabled={restartMutation.isPending}
                  onClick={() => restartMutation.mutate()}
                >
                  <RotateCw />
                  立即重启
                </Button>
              )}
            </div>
          ) : (
            <div className='text-right text-xs text-muted-foreground'>
              {value?.checkedAt ? formatDateTime(value.checkedAt) : '未检查'}
            </div>
          )}
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function UpdateProgress({ update }: { update: UpdateRun }) {
  const progress = Math.max(0, Math.min(100, update.progress || 0))
  const isDone = update.status === 'ready' || update.status === 'success'
  const isError = update.status === 'error'

  return (
    <div className='space-y-2'>
      <div className='flex items-center justify-between gap-3 text-sm'>
        <span className='min-w-0 truncate font-medium'>
          {statusText(update)}
        </span>
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
          进度已完成，重启服务后切换到新版本。
        </div>
      )}
      {update.status === 'restarting' && (
        <div className='text-xs text-muted-foreground'>服务正在重启...</div>
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

function statusText(update: UpdateRun) {
  if (update.status === 'running') return update.message || '正在更新'
  if (update.status === 'ready') return '更新准备完成'
  if (update.status === 'restarting') return '正在重启服务'
  if (update.status === 'success') return '更新完成'
  if (update.status === 'error') return '更新失败'
  return update.message
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
