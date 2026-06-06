import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { z } from 'zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { zodResolver } from '@hookform/resolvers/zod'
import {
  CheckCircle2,
  Edit3,
  FileKey2,
  KeyRound,
  Loader2,
  LockKeyhole,
  MoreHorizontal,
  Plus,
  RefreshCw,
  Terminal,
  Trash2,
  Upload,
} from 'lucide-react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import {
  deleteServerConnection,
  listServerConnections,
  saveServerConnection,
  touchServerMonitor,
  updateServerAgent,
  type ServerAuthType,
  type ServerConnection,
} from '@/services/server-connections'
import { apiConfig } from '@/config/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'

const formSchema = z.object({
  name: z.string().trim().min(1, '请输入名称。'),
  group_name: z.string().trim(),
  host: z.string().trim().min(1, '请输入服务器 IP 或域名。'),
  port: z
    .string()
    .trim()
    .refine((value) => {
      const port = Number(value)
      return Number.isInteger(port) && port >= 1 && port <= 65535
    }, '端口必须在 1 到 65535 之间。'),
  username: z.string().trim().min(1, '请输入用户名。'),
  auth_type: z.enum(['password', 'key']),
  password: z.string(),
  private_key: z.string(),
  key_file_name: z.string(),
  expires_at: z.string(),
  clear_secret: z.boolean(),
})

type FormValues = z.infer<typeof formSchema>

const defaultValues: FormValues = {
  name: '',
  group_name: '',
  host: '',
  port: '22',
  username: 'root',
  auth_type: 'password',
  password: '',
  private_key: '',
  key_file_name: '',
  expires_at: '',
  clear_secret: false,
}

export function ServerConnectionSettings() {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<ServerConnection | null>(null)
  const [terminalServer, setTerminalServer] = useState<ServerConnection | null>(
    null
  )

  const query = useQuery({
    queryKey: ['server-connections'],
    queryFn: listServerConnections,
    refetchInterval: 15_000,
  })

  useEffect(() => {
    void touchServerMonitor()
    const timer = window.setInterval(() => {
      void touchServerMonitor()
    }, 10_000)
    return () => window.clearInterval(timer)
  }, [])

  const saveMutation = useMutation({
    mutationFn: saveServerConnection,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['server-connections'] })
      setOpen(false)
      setEditing(null)
      toast.success('服务器设置已保存，后台会自动安装采集脚本')
    },
    onError: () => {
      toast.error('服务器设置保存失败')
    },
  })

  const deleteMutation = useMutation({
    mutationFn: deleteServerConnection,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['server-connections'] })
      toast.success('服务器已删除')
    },
    onError: () => {
      toast.error('服务器删除失败')
    },
  })

  const updateAgentMutation = useMutation({
    mutationFn: updateServerAgent,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['server-connections'] })
      toast.success('Agent 已更新')
    },
    onError: () => {
      toast.error('Agent 更新失败')
    },
  })

  return (
    <div className='space-y-5'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div className='text-sm text-muted-foreground'>
          已配置 {query.data?.length ?? 0} 台服务器
        </div>
        <Button
          type='button'
          size='sm'
          onClick={() => {
            setEditing(null)
            setOpen(true)
          }}
        >
          <Plus />
          添加服务器
        </Button>
      </div>

      {query.data?.length ? (
        <div className='overflow-hidden rounded-md border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>名称</TableHead>
                <TableHead>主机</TableHead>
                <TableHead>用户</TableHead>
                <TableHead>认证</TableHead>
                <TableHead>到期</TableHead>
                <TableHead>采集</TableHead>
                <TableHead className='w-24 text-right'>操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {query.data.map((item) => (
                <TableRow key={item.id}>
                  <TableCell className='font-medium'>{item.name}</TableCell>
                  <TableCell className='font-mono text-xs'>
                    {item.host}:{item.port}
                  </TableCell>
                  <TableCell>{item.username}</TableCell>
                  <TableCell>
                    <CredentialBadge
                      type={item.auth_type}
                      active={item.has_password || item.has_private_key}
                    />
                  </TableCell>
                  <TableCell>{formatDate(item.expires_at)}</TableCell>
                  <TableCell>
                    <CollectBadge item={item} />
                  </TableCell>
                  <TableCell>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          type='button'
                          variant='ghost'
                          size='icon'
                          aria-label='服务器操作'
                        >
                          <MoreHorizontal />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align='end' className='w-36'>
                        <DropdownMenuItem onClick={() => setTerminalServer(item)}>
                          <Terminal />
                          连接
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          disabled={updateAgentMutation.isPending}
                          onClick={() => updateAgentMutation.mutate(item.id)}
                        >
                          <RefreshCw />
                          更新 Agent
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          onClick={() => {
                            setEditing(item)
                            setOpen(true)
                          }}
                        >
                          <Edit3 />
                          编辑
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          variant='destructive'
                          disabled={deleteMutation.isPending}
                          onClick={() => deleteMutation.mutate(item.id)}
                        >
                          <Trash2 />
                          删除
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      ) : (
        <div className='flex min-h-48 items-center justify-center rounded-md border border-dashed'>
          <Button
            type='button'
            variant='outline'
            onClick={() => {
              setEditing(null)
              setOpen(true)
            }}
          >
            <Plus />
            添加第一台服务器
          </Button>
        </div>
      )}

      <ServerConnectionDialog
        open={open}
        item={editing}
        isSaving={saveMutation.isPending}
        onOpenChange={setOpen}
        onSave={(values) => {
          saveMutation.mutate({
            id: editing?.id,
            name: values.name.trim(),
            group_name: values.group_name.trim(),
            region: '',
            host: values.host.trim(),
            port: Number(values.port),
            username: values.username.trim(),
            auth_type: values.auth_type,
            expires_at: values.expires_at || undefined,
            ...(values.password ? { password: values.password } : {}),
            ...(values.private_key ? { private_key: values.private_key } : {}),
            ...(values.clear_secret ? { clear_secret: true } : {}),
          })
        }}
      />
      <ServerTerminalDialog
        server={terminalServer}
        onOpenChange={(open) => {
          if (!open) setTerminalServer(null)
        }}
      />
    </div>
  )
}

function CredentialBadge({
  type,
  active,
}: {
  type: ServerAuthType
  active: boolean
}) {
  const Icon = type === 'key' ? KeyRound : LockKeyhole
  return (
    <Badge variant={active ? 'default' : 'secondary'} className='gap-1'>
      <Icon className='size-3' />
      {type === 'key' ? '密钥登录' : '密码登录'}
    </Badge>
  )
}

function CollectBadge({ item }: { item: ServerConnection }) {
  if (item.collect_status === 'ok') {
    return (
      <Badge variant='outline' className='gap-1 text-emerald-600'>
        <CheckCircle2 className='size-3' />
        采集正常
      </Badge>
    )
  }
  if (item.collect_status === 'collecting') {
    return (
      <Badge variant='outline' className='gap-1'>
        <Loader2 className='size-3 animate-spin' />
        采集中
      </Badge>
    )
  }
  if (item.collect_status === 'error') {
    return <Badge variant='secondary'>待采集</Badge>
  }
  return <Badge variant='secondary'>等待采集</Badge>
}

function ServerTerminalDialog({
  server,
  onOpenChange,
}: {
  server: ServerConnection | null
  onOpenChange: (open: boolean) => void
}) {
  const [input, setInput] = useState('')
  const [output, setOutput] = useState('')
  const [status, setStatus] = useState<'idle' | 'connecting' | 'open' | 'closed'>(
    'idle'
  )
  const socketRef = useRef<WebSocket | null>(null)
  const outputRef = useRef<HTMLTextAreaElement | null>(null)

  useEffect(() => {
    if (!server) return
    const socket = new WebSocket(buildWebSSHUrl(server.id))
    socketRef.current = socket
    setInput('')
    setOutput('')
    setStatus('connecting')

    socket.onopen = () => {
      setStatus('open')
    }
    socket.onmessage = (event) => {
      const message = String(event.data)
      setOutput((current) => current + message)
      if (
        message.includes('连接失败') ||
        message.includes('打开输入失败') ||
        message.includes('打开输出失败') ||
        message.includes('打开错误输出失败') ||
        message.includes('启动 Shell 失败')
      ) {
        toast.error(message.trim())
      }
    }
    socket.onerror = () => {
      const message = 'SSH 连接异常，请检查服务器地址、端口、凭据或网络。'
      setOutput((current) => current + `\r\n${message}\r\n`)
      toast.error(message)
    }
    socket.onclose = () => {
      setStatus('closed')
    }

    return () => {
      socket.close()
      socketRef.current = null
    }
  }, [server])

  useEffect(() => {
    const element = outputRef.current
    if (element) {
      element.scrollTop = element.scrollHeight
    }
  }, [output])

  function sendInput() {
    const socket = socketRef.current
    if (!input || !socket || socket.readyState !== WebSocket.OPEN) return
    socket.send(input + '\n')
    setInput('')
  }

  return (
    <Dialog open={!!server} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[92vh] overflow-y-auto sm:max-w-4xl'>
        <DialogHeader>
          <DialogTitle>
            SSH - {server?.connection_hint}
            <span className='ml-3 text-xs font-normal text-muted-foreground'>
              {status === 'connecting'
                ? '连接中'
                : status === 'open'
                  ? '已连接'
                  : status === 'closed'
                    ? '已断开'
                    : ''}
            </span>
          </DialogTitle>
        </DialogHeader>
        <div className='space-y-3'>
          <Textarea
            ref={outputRef}
            readOnly
            value={output || '正在连接 SSH...'}
            className='min-h-[420px] resize-none bg-black font-mono text-xs text-green-100'
          />
          <div className='flex gap-2'>
            <Input
              value={input}
              onChange={(event) => setInput(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault()
                  sendInput()
                }
              }}
              className='font-mono'
              placeholder='输入命令，回车发送'
            />
            <Button
              type='button'
              disabled={status !== 'open'}
              onClick={sendInput}
            >
              <Terminal />
              发送
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function buildWebSSHUrl(id: string) {
  const base = new URL(apiConfig.baseURL, window.location.origin)
  base.protocol = base.protocol === 'https:' ? 'wss:' : 'ws:'
  base.pathname = `${base.pathname.replace(/\/$/, '')}/server-connections/${id}/ssh/ws`
  base.search = ''
  return base.toString()
}

function ServerConnectionDialog({
  open,
  item,
  isSaving,
  onOpenChange,
  onSave,
}: {
  open: boolean
  item: ServerConnection | null
  isSaving: boolean
  onOpenChange: (open: boolean) => void
  onSave: (values: FormValues) => void
}) {
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const form = useForm<FormValues>({
    resolver: zodResolver(formSchema),
    defaultValues,
  })
  const authType = form.watch('auth_type')
  const keyFileName = form.watch('key_file_name')

  useEffect(() => {
    if (!open) return
    form.reset(
      item
        ? {
            name: item.name,
            group_name: item.group_name,
            host: item.host,
            port: String(item.port || 22),
            username: item.username,
            auth_type: item.auth_type,
            password: '',
            private_key: '',
            key_file_name: item.has_private_key ? '已保存私钥' : '',
            expires_at: item.expires_at ? item.expires_at.slice(0, 10) : '',
            clear_secret: false,
          }
        : defaultValues
    )
  }, [form, item, open])

  async function handleKeyFile(file: File | undefined) {
    if (!file) return
    const text = await file.text()
    form.setValue('private_key', text, { shouldDirty: true })
    form.setValue('key_file_name', file.name, { shouldDirty: true })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[92vh] overflow-y-auto sm:max-w-3xl'>
        <DialogHeader>
          <DialogTitle>{item ? '编辑服务器' : '添加服务器'}</DialogTitle>
        </DialogHeader>

        <Form {...form}>
          <form
            id='server-connection-form'
            onSubmit={form.handleSubmit(onSave)}
            className='space-y-6'
          >
            <section className='space-y-3'>
              <div className='flex items-center gap-2 text-sm font-medium'>
                <FileKey2 className='size-4 text-muted-foreground' />
                基础信息
              </div>
              <div className='grid gap-4 md:grid-cols-2'>
                <TextField control={form.control} name='name' label='名称' />
                <TextField
                  control={form.control}
                  name='group_name'
                  label='分组'
                  placeholder='默认'
                />
              </div>
              <div className='grid gap-4 md:grid-cols-[minmax(0,1fr)_9rem]'>
                <TextField
                  control={form.control}
                  name='host'
                  label='服务器 IP / 域名'
                />
                <TextField
                  control={form.control}
                  name='port'
                  label='端口'
                  type='number'
                />
              </div>
              <div className='grid gap-4 md:grid-cols-2'>
                <TextField
                  control={form.control}
                  name='username'
                  label='用户名'
                />
                <TextField
                  control={form.control}
                  name='expires_at'
                  label='到期时间'
                  placeholder='2026-06-30'
                />
              </div>
            </section>

            <section className='space-y-3'>
              <div className='flex items-center gap-2 text-sm font-medium'>
                <LockKeyhole className='size-4 text-muted-foreground' />
                登录凭据
              </div>
              <div className='grid gap-4 md:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='auth_type'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>登录方式</FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={(value) => field.onChange(value)}
                      >
                        <FormControl>
                          <SelectTrigger className='w-full'>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value='password'>密码</SelectItem>
                          <SelectItem value='key'>密钥</SelectItem>
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                {authType === 'password' ? (
                  <TextField
                    control={form.control}
                    name='password'
                    label='密码'
                    type='password'
                    placeholder={item?.has_password ? '留空则保持原密码' : ''}
                  />
                ) : (
                  <FormField
                    control={form.control}
                    name='private_key'
                    render={() => (
                      <FormItem>
                        <FormLabel>私钥文件</FormLabel>
                        <FormControl>
                          <div className='flex min-h-9 items-center gap-2'>
                            <input
                              ref={fileInputRef}
                              type='file'
                              className='hidden'
                              onChange={(event) =>
                                handleKeyFile(event.target.files?.[0])
                              }
                            />
                            <Button
                              type='button'
                              variant='outline'
                              onClick={() => fileInputRef.current?.click()}
                            >
                              <Upload />
                              选择文件
                            </Button>
                            <div className='flex min-w-0 items-center gap-2 text-sm text-muted-foreground'>
                              <FileKey2 className='size-4 shrink-0' />
                              <span className='truncate'>
                                {keyFileName && keyFileName !== '已保存私钥'
                                  ? keyFileName
                                  : '未选择新文件'}
                              </span>
                            </div>
                          </div>
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                )}
              </div>
              {item && (item.has_password || item.has_private_key) && (
                <FormField
                  control={form.control}
                  name='clear_secret'
                  render={({ field }) => (
                    <FormItem className='flex flex-row items-center gap-2 pt-1'>
                      <FormControl>
                        <Checkbox
                          checked={field.value}
                          onCheckedChange={(checked) =>
                            field.onChange(!!checked)
                          }
                        />
                      </FormControl>
                      <FormLabel className='text-muted-foreground'>
                        清除已保存凭据
                      </FormLabel>
                    </FormItem>
                  )}
                />
              )}
            </section>
          </form>
        </Form>

        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            取消
          </Button>
          <Button
            type='submit'
            form='server-connection-form'
            disabled={isSaving}
          >
            保存
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function TextField({
  control,
  name,
  label,
  placeholder,
  type = 'text',
  icon,
}: {
  control: ReturnType<typeof useForm<FormValues>>['control']
  name: keyof FormValues
  label: string
  placeholder?: string
  type?: string
  icon?: ReactNode
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <div className='relative'>
              {icon && (
                <span className='pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground [&_svg]:size-4'>
                  {icon}
                </span>
              )}
              <Input
                type={type}
                autoComplete='off'
                placeholder={placeholder}
                className={icon ? 'ps-9' : undefined}
                {...field}
                value={String(field.value ?? '')}
              />
            </div>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

function formatDate(value: string | null) {
  if (!value) return '未设置'
  return new Date(value).toLocaleDateString('zh-CN')
}
