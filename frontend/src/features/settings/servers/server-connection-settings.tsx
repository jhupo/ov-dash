import { useEffect, useMemo, useRef, useState } from 'react'
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
  Plus,
  Server,
  Trash2,
  Upload,
} from 'lucide-react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import {
  deleteServerConnection,
  listServerConnections,
  saveServerConnection,
  type ServerAuthType,
  type ServerConnection,
} from '@/services/server-connections'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

const formSchema = z.object({
  name: z.string().trim().min(1, '请输入名称。'),
  group_name: z.string().trim(),
  region: z.string().trim(),
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
  region: '',
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

  const query = useQuery({
    queryKey: ['server-connections'],
    queryFn: listServerConnections,
    refetchInterval: 15_000,
  })

  const groupedItems = useMemo(() => {
    return (query.data ?? []).reduce<Record<string, ServerConnection[]>>(
      (groups, item) => {
        const key = item.group_name || '默认'
        groups[key] = groups[key] ?? []
        groups[key].push(item)
        return groups
      },
      {}
    )
  }, [query.data])

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
        <div className='grid gap-3 lg:grid-cols-2'>
          {Object.entries(groupedItems).map(([group, items]) =>
            items.map((item) => (
              <Card key={item.id}>
                <CardContent className='p-4'>
                  <div className='flex items-start justify-between gap-4'>
                    <div className='min-w-0 space-y-2'>
                      <div className='flex min-w-0 items-center gap-2'>
                        <Server className='size-4 shrink-0 text-muted-foreground' />
                        <span className='truncate font-semibold'>
                          {item.name}
                        </span>
                        <Badge variant='secondary'>{group}</Badge>
                      </div>
                      <div className='grid gap-1 text-sm text-muted-foreground'>
                        <span className='truncate'>
                          {item.username}@{item.host}:{item.port}
                        </span>
                        <span className='truncate'>
                          {item.region || '未设置地区'} · 到期{' '}
                          {formatDate(item.expires_at)}
                        </span>
                      </div>
                    </div>
                    <div className='flex shrink-0 gap-1'>
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon'
                        onClick={() => {
                          setEditing(item)
                          setOpen(true)
                        }}
                        aria-label='编辑服务器'
                      >
                        <Edit3 />
                      </Button>
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon'
                        disabled={deleteMutation.isPending}
                        onClick={() => deleteMutation.mutate(item.id)}
                        aria-label='删除服务器'
                      >
                        <Trash2 className='text-destructive' />
                      </Button>
                    </div>
                  </div>
                  <div className='mt-4 flex flex-wrap gap-2'>
                    <CredentialBadge
                      type={item.auth_type}
                      active={item.has_password || item.has_private_key}
                    />
                    <CollectBadge item={item} />
                  </div>
                </CardContent>
              </Card>
            ))
          )}
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
            region: values.region.trim(),
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
            region: item.region,
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
                <Server className='size-4 text-muted-foreground' />
                基础信息
              </div>
              <div className='grid gap-4 md:grid-cols-3'>
                <TextField control={form.control} name='name' label='名称' />
                <TextField
                  control={form.control}
                  name='group_name'
                  label='分组'
                  placeholder='默认'
                />
                <TextField
                  control={form.control}
                  name='region'
                  label='地区'
                  placeholder='HK / US'
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
