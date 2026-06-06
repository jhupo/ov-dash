import { useEffect } from 'react'
import { z } from 'zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import {
  getProxySettings,
  updateProxySettings,
} from '@/services/proxy-settings'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'

const proxyFormSchema = z
  .object({
    enabled: z.boolean(),
    host: z.string().trim(),
    port: z
      .string()
      .trim()
      .refine((value) => {
        const port = Number(value)
        return Number.isInteger(port) && port >= 1 && port <= 65535
      }, '端口必须在 1 到 65535 之间。'),
    username: z.string().trim(),
    password: z.string(),
    clearPassword: z.boolean(),
  })
  .refine((data) => !data.enabled || data.host.length > 0, {
    message: '启用代理后必须填写主机地址。',
    path: ['host'],
  })

type ProxyFormValues = z.infer<typeof proxyFormSchema>

const defaultValues: ProxyFormValues = {
  enabled: false,
  host: '',
  port: '1080',
  username: '',
  password: '',
  clearPassword: false,
}

export function ProxyForm() {
  const queryClient = useQueryClient()
  const proxySettings = useQuery({
    queryKey: ['proxy-settings'],
    queryFn: getProxySettings,
  })

  const form = useForm<ProxyFormValues>({
    resolver: zodResolver(proxyFormSchema),
    defaultValues,
  })

  useEffect(() => {
    if (!proxySettings.data) return

    form.reset({
      enabled: proxySettings.data.enabled,
      host: proxySettings.data.host,
      port: String(proxySettings.data.port || 1080),
      username: proxySettings.data.username,
      password: '',
      clearPassword: false,
    })
  }, [form, proxySettings.data])

  const mutation = useMutation({
    mutationFn: updateProxySettings,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['proxy-settings'] })
      form.reset({ ...form.getValues(), password: '' })
      toast.success('代理设置已更新')
    },
    onError: () => {
      toast.error('代理设置保存失败')
    },
  })

  function onSubmit(data: ProxyFormValues) {
    mutation.mutate({
      enabled: data.enabled,
      host: data.host.trim(),
      port: Number(data.port),
      username: data.username.trim(),
      ...(data.password ? { password: data.password } : {}),
      ...(!data.password && data.clearPassword ? { clear_password: true } : {}),
    })
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className='space-y-6'>
        <FormField
          control={form.control}
          name='enabled'
          render={({ field }) => (
            <FormItem className='flex items-center justify-between gap-4'>
              <FormLabel>启用 SOCKS5 代理</FormLabel>
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                  aria-label='启用 SOCKS5 代理'
                />
              </FormControl>
            </FormItem>
          )}
        />

        <div className='grid gap-4 sm:grid-cols-[minmax(0,1fr)_9rem]'>
          <FormField
            control={form.control}
            name='host'
            render={({ field }) => (
              <FormItem>
                <FormLabel>主机</FormLabel>
                <FormControl>
                  <Input placeholder='127.0.0.1' autoComplete='off' {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='port'
            render={({ field }) => (
              <FormItem>
                <FormLabel>端口</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={1}
                    max={65535}
                    inputMode='numeric'
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>

        <div className='grid gap-4 sm:grid-cols-2'>
          <FormField
            control={form.control}
            name='username'
            render={({ field }) => (
              <FormItem>
                <FormLabel>用户名</FormLabel>
                <FormControl>
                  <Input autoComplete='off' {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='password'
            render={({ field }) => (
              <FormItem>
                <FormLabel>密码</FormLabel>
                <FormControl>
                  <Input
                    type='password'
                    autoComplete='new-password'
                    placeholder={
                      proxySettings.data?.has_password
                        ? '留空则保持原密码'
                        : ''
                    }
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>

        {proxySettings.data?.has_password && (
          <FormField
            control={form.control}
            name='clearPassword'
            render={({ field }) => (
              <FormItem className='flex flex-row items-start'>
                <FormControl>
                  <Checkbox
                    checked={field.value}
                    onCheckedChange={(checked) => field.onChange(!!checked)}
                  />
                </FormControl>
                <div className='space-y-1 leading-none'>
                  <FormLabel>清除已保存密码</FormLabel>
                </div>
              </FormItem>
            )}
          />
        )}

        <Button
          type='submit'
          disabled={proxySettings.isLoading || mutation.isPending}
        >
          保存代理设置
        </Button>
      </form>
    </Form>
  )
}
