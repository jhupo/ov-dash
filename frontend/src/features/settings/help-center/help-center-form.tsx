import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/ui/tabs'
import {
  DEFAULT_HELP_DOCUMENT,
  readHelpDocument,
  resetHelpDocument,
  saveHelpDocument,
} from '@/features/help-center/help-document'
import { MarkdownViewer } from '@/features/help-center/markdown-viewer'

export function HelpCenterForm() {
  const [markdown, setMarkdown] = useState(readHelpDocument)

  useEffect(() => {
    setMarkdown(readHelpDocument())
  }, [])

  const handleSave = () => {
    saveHelpDocument(markdown)
    toast.success('帮助中心内容已保存')
  }

  const handleReset = () => {
    resetHelpDocument()
    setMarkdown(DEFAULT_HELP_DOCUMENT)
    toast.success('帮助中心内容已恢复默认')
  }

  return (
    <div className='space-y-4'>
      <Tabs defaultValue='edit' className='w-full'>
        <TabsList>
          <TabsTrigger value='edit'>编辑</TabsTrigger>
          <TabsTrigger value='preview'>预览</TabsTrigger>
        </TabsList>
        <TabsContent value='edit'>
          <Textarea
            value={markdown}
            onChange={(event) => setMarkdown(event.target.value)}
            className='min-h-[520px] resize-y font-mono text-sm leading-6'
            spellCheck={false}
          />
        </TabsContent>
        <TabsContent value='preview'>
          <div className='min-h-[520px] rounded-md border p-4'>
            <MarkdownViewer markdown={markdown} />
          </div>
        </TabsContent>
      </Tabs>
      <div className='flex flex-wrap gap-2'>
        <Button type='button' onClick={handleSave}>
          保存帮助中心
        </Button>
        <Button type='button' variant='outline' onClick={handleReset}>
          恢复默认
        </Button>
      </div>
    </div>
  )
}
