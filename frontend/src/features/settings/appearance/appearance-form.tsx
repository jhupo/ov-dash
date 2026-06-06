import { z } from 'zod'
import { useForm } from 'react-hook-form'
import { ChevronDownIcon } from '@radix-ui/react-icons'
import { zodResolver } from '@hookform/resolvers/zod'
import { fonts } from '@/config/fonts'
import { showSubmittedData } from '@/lib/show-submitted-data'
import { cn } from '@/lib/utils'
import {
  appLanguages,
  type AppLanguage,
  useAppPreferences,
} from '@/context/app-preferences-provider'
import { useFont } from '@/context/font-provider'
import { useTheme } from '@/context/theme-provider'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

const appearanceFormSchema = z.object({
  brandName: z.string().trim().min(1, '请输入应用名称。').max(24),
  subtitle: z.string().trim().min(1, '请输入侧栏副标题。').max(32),
  language: z.enum(['zh-CN', 'en-US']),
  theme: z.enum(['system', 'light', 'dark']),
  font: z.enum(fonts),
})

type AppearanceFormValues = z.infer<typeof appearanceFormSchema>

export function AppearanceForm() {
  const { font, setFont } = useFont()
  const { theme, setTheme } = useTheme()
  const { brandName, language, setPreferences, subtitle } = useAppPreferences()

  const defaultValues: Partial<AppearanceFormValues> = {
    brandName,
    font,
    language,
    subtitle,
    theme,
  }

  const form = useForm<AppearanceFormValues>({
    resolver: zodResolver(appearanceFormSchema),
    defaultValues,
  })

  function onSubmit(data: AppearanceFormValues) {
    if (data.font != font) setFont(data.font)
    if (data.theme != theme) setTheme(data.theme)
    if (
      data.brandName != brandName ||
      data.subtitle != subtitle ||
      data.language != language
    ) {
      setPreferences({
        brandName: data.brandName.trim(),
        subtitle: data.subtitle.trim(),
        language: data.language,
      })
    }

    showSubmittedData(data)
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className='space-y-8'>
        <div className='grid max-w-2xl gap-4 sm:grid-cols-2'>
          <FormField
            control={form.control}
            name='brandName'
            render={({ field }) => (
              <FormItem>
                <FormLabel>应用名称</FormLabel>
                <FormControl>
                  <Input placeholder='OV Dash' {...field} />
                </FormControl>
                <FormDescription>
                  显示在侧边栏左上角的主标题。
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='subtitle'
            render={({ field }) => (
              <FormItem>
                <FormLabel>侧栏副标题</FormLabel>
                <FormControl>
                  <Input placeholder='运营仪表盘' {...field} />
                </FormControl>
                <FormDescription>
                  显示在应用名称下方的说明文字。
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>
        <FormField
          control={form.control}
          name='language'
          render={({ field }) => (
            <FormItem>
              <FormLabel>语言</FormLabel>
              <Select
                onValueChange={(value) => field.onChange(value as AppLanguage)}
                defaultValue={field.value}
              >
                <FormControl>
                  <SelectTrigger className='w-50'>
                    <SelectValue placeholder='选择界面语言' />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  {appLanguages.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FormDescription>
                设置界面的语言偏好，支持简体中文和 English。
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='font'
          render={({ field }) => (
            <FormItem>
              <FormLabel>字体</FormLabel>
              <div className='relative w-max'>
                <FormControl>
                  <select
                    className={cn(
                      buttonVariants({ variant: 'outline' }),
                      'w-50 appearance-none font-normal capitalize',
                      'dark:bg-background dark:hover:bg-background'
                    )}
                    {...field}
                  >
                    {fonts.map((font) => (
                      <option key={font} value={font}>
                        {font}
                      </option>
                    ))}
                  </select>
                </FormControl>
                <ChevronDownIcon className='absolute inset-e-3 top-2.5 h-4 w-4 opacity-50' />
              </div>
              <FormDescription className='font-manrope'>
                设置仪表盘使用的字体。
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='theme'
          render={({ field }) => (
            <FormItem>
              <FormLabel>主题</FormLabel>
              <FormDescription>
                选择仪表盘主题。
              </FormDescription>
              <FormMessage />
              <RadioGroup
                onValueChange={field.onChange}
                defaultValue={field.value}
                className='grid max-w-2xl grid-cols-1 gap-6 pt-2 sm:grid-cols-3'
              >
                <FormItem>
                  <FormLabel className='[&:has([data-state=checked])>div]:border-primary'>
                    <FormControl>
                      <RadioGroupItem value='system' className='sr-only' />
                    </FormControl>
                    <div className='items-center rounded-md border-2 border-muted p-1 hover:border-accent'>
                      <div className='grid grid-cols-2 gap-1 rounded-sm bg-[#ecedef] p-2'>
                        <div className='space-y-2 rounded-sm bg-white p-2 shadow-xs'>
                          <div className='h-2 w-12 rounded-lg bg-[#ecedef]' />
                          <div className='h-2 w-16 rounded-lg bg-[#ecedef]' />
                        </div>
                        <div className='space-y-2 rounded-sm bg-slate-800 p-2 shadow-xs'>
                          <div className='h-2 w-12 rounded-lg bg-slate-400' />
                          <div className='h-2 w-16 rounded-lg bg-slate-400' />
                        </div>
                        <div className='h-2 rounded-lg bg-white' />
                        <div className='h-2 rounded-lg bg-slate-800' />
                      </div>
                    </div>
                    <span className='block w-full p-2 text-center font-normal'>
                      跟随系统
                    </span>
                  </FormLabel>
                </FormItem>
                <FormItem>
                  <FormLabel className='[&:has([data-state=checked])>div]:border-primary'>
                    <FormControl>
                      <RadioGroupItem value='light' className='sr-only' />
                    </FormControl>
                    <div className='items-center rounded-md border-2 border-muted p-1 hover:border-accent'>
                      <div className='space-y-2 rounded-sm bg-[#ecedef] p-2'>
                        <div className='space-y-2 rounded-md bg-white p-2 shadow-xs'>
                          <div className='h-2 w-20 rounded-lg bg-[#ecedef]' />
                          <div className='h-2 w-25 rounded-lg bg-[#ecedef]' />
                        </div>
                        <div className='flex items-center space-x-2 rounded-md bg-white p-2 shadow-xs'>
                          <div className='h-4 w-4 rounded-full bg-[#ecedef]' />
                          <div className='h-2 w-25 rounded-lg bg-[#ecedef]' />
                        </div>
                        <div className='flex items-center space-x-2 rounded-md bg-white p-2 shadow-xs'>
                          <div className='h-4 w-4 rounded-full bg-[#ecedef]' />
                          <div className='h-2 w-25 rounded-lg bg-[#ecedef]' />
                        </div>
                      </div>
                    </div>
                    <span className='block w-full p-2 text-center font-normal'>
                      浅色
                    </span>
                  </FormLabel>
                </FormItem>
                <FormItem>
                  <FormLabel className='[&:has([data-state=checked])>div]:border-primary'>
                    <FormControl>
                      <RadioGroupItem value='dark' className='sr-only' />
                    </FormControl>
                    <div className='items-center rounded-md border-2 border-muted bg-popover p-1 hover:bg-accent hover:text-accent-foreground'>
                      <div className='space-y-2 rounded-sm bg-slate-950 p-2'>
                        <div className='space-y-2 rounded-md bg-slate-800 p-2 shadow-xs'>
                          <div className='h-2 w-20 rounded-lg bg-slate-400' />
                          <div className='h-2 w-25 rounded-lg bg-slate-400' />
                        </div>
                        <div className='flex items-center space-x-2 rounded-md bg-slate-800 p-2 shadow-xs'>
                          <div className='h-4 w-4 rounded-full bg-slate-400' />
                          <div className='h-2 w-25 rounded-lg bg-slate-400' />
                        </div>
                        <div className='flex items-center space-x-2 rounded-md bg-slate-800 p-2 shadow-xs'>
                          <div className='h-4 w-4 rounded-full bg-slate-400' />
                          <div className='h-2 w-25 rounded-lg bg-slate-400' />
                        </div>
                      </div>
                    </div>
                    <span className='block w-full p-2 text-center font-normal'>
                      深色
                    </span>
                  </FormLabel>
                </FormItem>
              </RadioGroup>
            </FormItem>
          )}
        />

        <Button type='submit'>更新偏好</Button>
      </form>
    </Form>
  )
}
