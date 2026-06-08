import { useEffect } from 'react'
import { z } from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  getProxySettings,
  updateProxySettings,
} from '@/services/proxy-settings'
import { toast } from 'sonner'
import { useCan } from '@/hooks/use-can'
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

const proxyFormSchema = z.object({
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

type ProxyFormValues = z.infer<typeof proxyFormSchema>

const defaultValues: ProxyFormValues = {
  host: '',
  port: '1080',
  username: '',
  password: '',
  clearPassword: false,
}

export function ProxyForm() {
  const queryClient = useQueryClient()
  const can = useCan()
  const canWriteProxy = can('proxy:write')
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
    if (!canWriteProxy) return

    const host = data.host.trim()

    mutation.mutate({
      enabled: host.length > 0,
      host,
      port: Number(data.port),
      username: data.username.trim(),
      ...(data.password ? { password: data.password } : {}),
      ...(!data.password && data.clearPassword ? { clear_password: true } : {}),
    })
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className='space-y-5'>
        <div className='grid gap-4 md:grid-cols-[minmax(0,1fr)_10rem]'>
          <FormField
            control={form.control}
            name='host'
            render={({ field }) => (
              <FormItem>
                <FormLabel>主机</FormLabel>
                <FormControl>
                  <Input
                    placeholder='127.0.0.1'
                    autoComplete='off'
                    {...field}
                  />
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

        <div className='grid gap-4 md:grid-cols-2'>
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
                      proxySettings.data?.has_password ? '留空则保持原密码' : ''
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
          disabled={
            !canWriteProxy || proxySettings.isLoading || mutation.isPending
          }
        >
          保存代理设置
        </Button>
      </form>
    </Form>
  )
}
