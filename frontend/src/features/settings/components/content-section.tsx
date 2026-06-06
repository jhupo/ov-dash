import { Separator } from '@/components/ui/separator'

type ContentSectionProps = {
  title: string
  desc: string
  children: React.JSX.Element
  showHeader?: boolean
  size?: 'default' | 'wide'
}

export function ContentSection({
  title,
  desc,
  children,
  showHeader = false,
  size = 'default',
}: ContentSectionProps) {
  return (
    <div className='flex flex-1 flex-col'>
      {showHeader && (
        <>
          <div className='flex-none'>
            <h3 className='text-lg font-medium'>{title}</h3>
            <p className='text-sm text-muted-foreground'>{desc}</p>
          </div>
          <Separator className='my-4 flex-none' />
        </>
      )}
      <div className='faded-bottom h-full w-full overflow-y-auto scroll-smooth px-1 pb-12'>
        <div
          className={
            size === 'wide'
              ? 'mx-auto w-full max-w-4xl'
              : 'mx-auto w-full max-w-2xl'
          }
        >
          {children}
        </div>
      </div>
    </div>
  )
}
