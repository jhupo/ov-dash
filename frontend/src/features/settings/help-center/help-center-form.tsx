import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import {
  DEFAULT_HELP_DOCUMENT,
  readHelpDocument,
  resetHelpDocument,
  saveHelpDocument,
} from '@/features/help-center/help-document'

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
      <Textarea
        value={markdown}
        onChange={(event) => setMarkdown(event.target.value)}
        className='min-h-[560px] resize-y font-mono text-sm leading-6'
        spellCheck={false}
      />
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
