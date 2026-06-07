import {
  type ChangeEvent,
  type ClipboardEvent,
  type Dispatch,
  type DragEvent,
  type SetStateAction,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import axios from 'axios'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  createWikiPage,
  deleteWikiPage,
  getWikiPages,
  updateWikiPage,
  uploadWikiAttachment,
  type SaveWikiPagePayload,
  type SaveWikiResourcePayload,
  type WikiPage,
  type WikiPageType,
  type WikiResource,
  type WikiResourceType,
} from '@/services/wiki'
import {
  ArrowDown,
  ArrowUp,
  BookOpen,
  ChevronDown,
  ChevronUp,
  Copy,
  ExternalLink,
  FileText,
  GripVertical,
  ImagePlus,
  KeyRound,
  Link2,
  Monitor,
  Pencil,
  Plus,
  Save,
  SearchIcon,
  Table2,
  Trash2,
  Wrench,
  X,
} from 'lucide-react'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { ConfigDrawer } from '@/components/config-drawer'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'
import { MarkdownViewer } from '@/features/help-center/markdown-viewer'

type DraftWikiResource = SaveWikiResourcePayload & {
  draft_id: string
}

type DraftWikiPage = Omit<SaveWikiPagePayload, 'resources'> & {
  resources: DraftWikiResource[]
}

const pageTypeOptions: {
  label: string
  value: WikiPageType
  icon: typeof FileText
}[] = [
  { label: '文档', value: 'document', icon: FileText },
  { label: '机器', value: 'machine', icon: Monitor },
  { label: '链接', value: 'link', icon: Link2 },
  { label: '手册', value: 'runbook', icon: BookOpen },
  { label: '故障', value: 'troubleshooting', icon: Wrench },
]

const resourceTypeOptions: {
  label: string
  value: WikiResourceType
  icon: typeof FileText
}[] = [
  { label: '机器信息', value: 'machine', icon: Monitor },
  { label: '链接', value: 'link', icon: Link2 },
  { label: '账号密码', value: 'credential', icon: KeyRound },
  { label: '备注', value: 'note', icon: FileText },
]

const categoryOptions = [
  '机器信息',
  '操作手册',
  '故障排查',
  '外部链接',
  '账号资料',
]

const runbookTemplate = `# 操作手册标题

## 适用场景

## 前置条件

## 操作步骤

1. 待补充

## 验证方式

## 回滚方案
`

const troubleshootingTemplate = `# 故障标题

## 现象

## 影响范围

## 原因

## 处理步骤

1. 待补充

## 验证方式

## 后续跟进
`

function tableTemplate(rows: number, columns: number) {
  const safeRows = Math.min(Math.max(rows, 1), 20)
  const safeColumns = Math.min(Math.max(columns, 1), 10)
  const headers = Array.from(
    { length: safeColumns },
    (_, index) => `列 ${index + 1}`
  )
  const divider = Array.from({ length: safeColumns }, () => '---')
  const body = Array.from({ length: safeRows }, () =>
    Array.from({ length: safeColumns }, () => ' ')
  )
  return [headers, divider, ...body]
    .map((row) => `| ${row.join(' | ')} |`)
    .join('\n')
}

function draftID() {
  return Math.random().toString(36).slice(2)
}

function emptyDraft(): DraftWikiPage {
  return {
    parent_id: '',
    title: '',
    page_type: 'document',
    category: '机器信息',
    summary: '',
    content_md: '',
    tags: '',
    resources: [],
  }
}

function emptyResource(resourceType: WikiResourceType): DraftWikiResource {
  return {
    draft_id: draftID(),
    resource_type: resourceType,
    title: '',
    host: '',
    port: resourceType === 'machine' ? '22' : '',
    url: resourceType === 'link' ? 'https://' : '',
    username: '',
    password: '',
    note: '',
    sort_order: 0,
  }
}

function draftFromPage(page: WikiPage): DraftWikiPage {
  return {
    parent_id: page.parent_id,
    title: page.title,
    page_type: page.page_type,
    category: page.category,
    summary: page.summary,
    content_md: page.content_md,
    tags: page.tags,
    resources: (page.resources ?? []).map((resource) => ({
      draft_id: resource.id || draftID(),
      id: resource.id,
      resource_type: resource.resource_type,
      title: resource.title,
      host: resource.host,
      port: resource.port,
      url: resource.url,
      username: resource.username,
      password: resource.password,
      note: resource.note,
      sort_order: resource.sort_order,
    })),
  }
}

function pageTypeLabel(value: WikiPageType) {
  return pageTypeOptions.find((item) => item.value === value)?.label ?? '文档'
}

function pageTypeIcon(value: WikiPageType) {
  return pageTypeOptions.find((item) => item.value === value)?.icon ?? FileText
}

function resourceTypeLabel(value: WikiResourceType) {
  return (
    resourceTypeOptions.find((item) => item.value === value)?.label ?? '资源'
  )
}

function resourceTypeIcon(value: WikiResourceType) {
  return (
    resourceTypeOptions.find((item) => item.value === value)?.icon ?? FileText
  )
}

function normalizeDraft(draft: DraftWikiPage): SaveWikiPagePayload {
  return {
    parent_id: draft.parent_id?.trim() || '',
    title: draft.title.trim(),
    page_type: draft.page_type,
    category: draft.category.trim() || '资料库',
    summary: draft.summary.trim(),
    content_md: draft.content_md,
    tags: draft.tags.trim(),
    resources: draft.resources.map((resource, index) => ({
      id: resource.id,
      resource_type: resource.resource_type,
      title: resource.title.trim(),
      host: resource.host.trim(),
      port: resource.port.trim(),
      url: resource.url.trim(),
      username: resource.username.trim(),
      password: resource.password.trim(),
      note: resource.note.trim(),
      sort_order: index + 1,
    })),
  }
}

function copyValueFallback(value: string) {
  const textarea = document.createElement('textarea')
  textarea.value = value
  textarea.setAttribute('readonly', '')
  textarea.style.position = 'fixed'
  textarea.style.top = '-9999px'
  textarea.style.opacity = '0'

  document.body.appendChild(textarea)
  textarea.focus()
  textarea.select()

  try {
    return document.execCommand('copy')
  } finally {
    document.body.removeChild(textarea)
  }
}

async function copyValue(value: string, label: string) {
  if (!value) return
  try {
    const copied = copyValueFallback(value)
    if (copied) {
      toast.success(`${label}已复制`)
      return
    }

    if (window.isSecureContext && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value)
      toast.success(`${label}已复制`)
      return
    }
  } catch {
    // Try the toast below after both clipboard paths have failed.
  }
  toast.error('复制失败')
}

function imageUploadErrorMessage(error: unknown) {
  if (axios.isAxiosError(error)) {
    if (error.response?.status === 404) {
      return '图片上传接口未连接，请启动后端或配置 API 地址'
    }
    if (error.response?.status === 401) {
      return '登录已失效，请重新登录后上传'
    }
    if (error.response?.status === 413) {
      return '图片超过 10MB'
    }
    if (error.response?.status === 415) {
      return '仅支持 png、jpg、gif、webp'
    }
    if (!error.response) {
      return '无法连接到后端上传接口'
    }
  }
  return '图片上传失败'
}

export function Wiki() {
  const queryClient = useQueryClient()
  const [searchValue, setSearchValue] = useState('')
  const [selectedID, setSelectedID] = useState('')
  const [draft, setDraft] = useState<DraftWikiPage>(emptyDraft)
  const [isEditing, setIsEditing] = useState(false)
  const [isCreating, setIsCreating] = useState(false)

  const pagesQuery = useQuery({
    queryKey: ['wiki', 'pages'],
    queryFn: getWikiPages,
  })

  const pages = pagesQuery.data ?? []
  const selectedPage = pages.find((page) => page.id === selectedID)

  useEffect(() => {
    if (!selectedID && pages.length > 0 && !isCreating) {
      setSelectedID(pages[0].id)
    }
  }, [isCreating, pages, selectedID])

  useEffect(() => {
    if (selectedPage && !isCreating) {
      setDraft(draftFromPage(selectedPage))
      setIsEditing(false)
    }
  }, [isCreating, selectedPage])

  const filteredPages = useMemo(() => {
    const keyword = searchValue.trim().toLowerCase()
    if (!keyword) return pages

    return pages.filter((page) =>
      [
        page.title,
        page.category,
        page.summary,
        page.content_md,
        page.tags,
        ...(page.resources ?? []).flatMap((resource) => [
          resource.title,
          resource.host,
          resource.url,
          resource.username,
          resource.password,
          resource.note,
        ]),
      ]
        .join(' ')
        .toLowerCase()
        .includes(keyword)
    )
  }, [pages, searchValue])

  const groupedPages = useMemo(() => {
    const groups = new Map<string, WikiPage[]>()
    for (const page of filteredPages) {
      const key = page.category || '资料库'
      groups.set(key, [...(groups.get(key) ?? []), page])
    }
    return [...groups.entries()]
  }, [filteredPages])

  const createMutation = useMutation({
    mutationFn: createWikiPage,
    onSuccess: async (page) => {
      await queryClient.invalidateQueries({ queryKey: ['wiki', 'pages'] })
      setSelectedID(page.id)
      setIsCreating(false)
      setIsEditing(false)
      toast.success('资料已创建')
    },
    onError: () => toast.error('资料创建失败'),
  })

  const updateMutation = useMutation({
    mutationFn: ({
      id,
      payload,
    }: {
      id: string
      payload: SaveWikiPagePayload
    }) => updateWikiPage(id, payload),
    onSuccess: async (page) => {
      await queryClient.invalidateQueries({ queryKey: ['wiki', 'pages'] })
      setSelectedID(page.id)
      setIsEditing(false)
      toast.success('资料已保存')
    },
    onError: () => toast.error('资料保存失败'),
  })

  const deleteMutation = useMutation({
    mutationFn: deleteWikiPage,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['wiki', 'pages'] })
      setSelectedID('')
      setIsCreating(false)
      setIsEditing(false)
      toast.success('资料已删除')
    },
    onError: () => toast.error('资料删除失败'),
  })

  function startCreate(pageType: WikiPageType = 'document') {
    const nextDraft = { ...emptyDraft(), page_type: pageType }
    if (pageType === 'machine') {
      nextDraft.resources = [{ ...emptyResource('machine'), title: '机器信息' }]
    }
    setDraft(nextDraft)
    setSelectedID('')
    setIsCreating(true)
    setIsEditing(true)
  }

  function saveDraft() {
    const payload = normalizeDraft(draft)
    if (!payload.title) {
      toast.error('请输入标题')
      return
    }
    if (isCreating) {
      createMutation.mutate(payload)
      return
    }
    if (!selectedPage) return
    updateMutation.mutate({ id: selectedPage.id, payload })
  }

  function removeSelectedPage() {
    if (!selectedPage) return
    if (!window.confirm(`删除“${selectedPage.title}”？`)) return
    deleteMutation.mutate(selectedPage.id)
  }

  const isSaving = createMutation.isPending || updateMutation.isPending

  return (
    <>
      <Header fixed>
        <Search className='me-auto' />
        <ThemeSwitch />
        <ConfigDrawer />
        <ProfileDropdown />
      </Header>

      <Main className='flex flex-1 flex-col gap-4 overflow-hidden'>
        <div className='flex flex-wrap items-end justify-between gap-3'>
          <div>
            <h2 className='text-2xl font-bold tracking-tight'>资料库</h2>
            <p className='text-muted-foreground'>
              机器信息、操作手册、故障指导和外部链接。
            </p>
          </div>
          <div className='flex gap-2'>
            <Button
              type='button'
              variant='outline'
              onClick={() => startCreate('machine')}
            >
              <Monitor className='me-2 size-4' />
              机器
            </Button>
            <Button type='button' onClick={() => startCreate()}>
              <Plus className='me-2 size-4' />
              新建
            </Button>
          </div>
        </div>

        <div className='grid min-h-0 flex-1 gap-4 lg:grid-cols-[320px_minmax(0,1fr)]'>
          <aside className='flex min-h-0 flex-col rounded-md border bg-background'>
            <div className='border-b p-3'>
              <div className='relative'>
                <SearchIcon className='pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground' />
                <Input
                  value={searchValue}
                  onChange={(event) => setSearchValue(event.target.value)}
                  className='ps-9'
                  placeholder='搜索资料'
                />
              </div>
            </div>
            <div className='min-h-0 flex-1 overflow-y-auto p-2'>
              {pagesQuery.isLoading && (
                <div className='px-3 py-8 text-center text-sm text-muted-foreground'>
                  正在加载...
                </div>
              )}
              {!pagesQuery.isLoading && groupedPages.length === 0 && (
                <div className='px-3 py-8 text-center text-sm text-muted-foreground'>
                  暂无资料
                </div>
              )}
              {groupedPages.map(([category, items]) => (
                <div key={category} className='mb-4'>
                  <div className='mb-1 px-2 text-xs font-medium text-muted-foreground'>
                    {category}
                  </div>
                  <div className='space-y-1'>
                    {items.map((page) => {
                      const Icon = pageTypeIcon(page.page_type)
                      const summary =
                        page.summary ||
                        page.resources?.[0]?.title ||
                        page.tags ||
                        page.content_md
                      return (
                        <button
                          key={page.id}
                          type='button'
                          onClick={() => {
                            setSelectedID(page.id)
                            setIsCreating(false)
                          }}
                          className={cn(
                            'flex w-full items-start gap-2 rounded-md px-2 py-2 text-start text-sm transition-colors hover:bg-muted',
                            selectedID === page.id &&
                              !isCreating &&
                              'bg-muted text-foreground'
                          )}
                        >
                          <Icon className='mt-0.5 size-4 shrink-0 text-muted-foreground' />
                          <span className='min-w-0 flex-1'>
                            <span className='block truncate font-medium'>
                              {page.title}
                            </span>
                            <span className='line-clamp-2 text-xs text-muted-foreground'>
                              {summary}
                            </span>
                          </span>
                        </button>
                      )
                    })}
                  </div>
                </div>
              ))}
            </div>
          </aside>

          <section className='min-h-0 overflow-hidden rounded-md border bg-background'>
            {isEditing ? (
              <WikiEditor
                draft={draft}
                setDraft={setDraft}
                pageID={selectedID}
                isCreating={isCreating}
                isSaving={isSaving}
                onCancel={() => {
                  if (selectedPage) {
                    setDraft(draftFromPage(selectedPage))
                    setIsEditing(false)
                    setIsCreating(false)
                  } else {
                    setIsCreating(false)
                    setIsEditing(false)
                  }
                }}
                onSave={saveDraft}
              />
            ) : selectedPage ? (
              <WikiPageView
                page={selectedPage}
                onEdit={() => setIsEditing(true)}
                onDelete={removeSelectedPage}
                deleting={deleteMutation.isPending}
              />
            ) : (
              <div className='flex h-full min-h-[420px] items-center justify-center p-8 text-center text-sm text-muted-foreground'>
                选择或新建一条资料
              </div>
            )}
          </section>
        </div>
      </Main>
    </>
  )
}

function WikiPageView({
  page,
  onEdit,
  onDelete,
  deleting,
}: {
  page: WikiPage
  onEdit: () => void
  onDelete: () => void
  deleting: boolean
}) {
  const Icon = pageTypeIcon(page.page_type)
  const groupedResources = resourceTypeOptions
    .map((option) => ({
      ...option,
      items: (page.resources ?? []).filter(
        (resource) => resource.resource_type === option.value
      ),
    }))
    .filter((group) => group.items.length > 0)

  return (
    <div className='flex h-full min-h-0 flex-col'>
      <div className='flex flex-wrap items-start justify-between gap-3 border-b p-5'>
        <div className='min-w-0 space-y-2'>
          <div className='flex flex-wrap items-center gap-2'>
            <Badge variant='secondary'>
              <Icon className='size-3' />
              {pageTypeLabel(page.page_type)}
            </Badge>
            <Badge variant='outline'>{page.category}</Badge>
            {page.tags && <Badge variant='outline'>{page.tags}</Badge>}
          </div>
          <div>
            <h3 className='text-2xl font-semibold tracking-tight'>
              {page.title}
            </h3>
            {page.summary && (
              <p className='mt-1 text-sm text-muted-foreground'>
                {page.summary}
              </p>
            )}
          </div>
        </div>
        <div className='flex gap-2'>
          <Button type='button' variant='outline' onClick={onEdit}>
            <Pencil className='me-2 size-4' />
            编辑
          </Button>
          <Button
            type='button'
            variant='outline'
            disabled={deleting}
            onClick={onDelete}
          >
            <Trash2 className='me-2 size-4' />
            删除
          </Button>
        </div>
      </div>

      <div className='min-h-0 flex-1 overflow-y-auto p-5'>
        {groupedResources.length > 0 && (
          <div className='mb-6 space-y-5'>
            {groupedResources.map((group) => {
              const GroupIcon = group.icon
              return (
                <div key={group.value} className='space-y-2'>
                  <div className='flex items-center gap-2 text-sm font-medium'>
                    <GroupIcon className='size-4' />
                    {group.label}
                  </div>
                  <div className='grid gap-3 md:grid-cols-2'>
                    {group.items.map((resource) => (
                      <ResourceCard key={resource.id} resource={resource} />
                    ))}
                  </div>
                </div>
              )
            })}
          </div>
        )}

        {page.content_md ? (
          <MarkdownViewer markdown={page.content_md} />
        ) : (
          <div className='rounded-md border border-dashed p-8 text-center text-sm text-muted-foreground'>
            暂无正文
          </div>
        )}
      </div>
    </div>
  )
}

function ResourceCard({ resource }: { resource: WikiResource }) {
  const Icon = resourceTypeIcon(resource.resource_type)

  return (
    <div className='space-y-3 rounded-md border p-3 text-sm'>
      <div className='flex items-start justify-between gap-2'>
        <div className='flex min-w-0 items-center gap-2 font-medium'>
          <Icon className='size-4 shrink-0' />
          <span className='truncate'>{resource.title}</span>
        </div>
        {resource.url && (
          <a
            href={resource.url}
            target='_blank'
            rel='noreferrer'
            className='inline-flex shrink-0 items-center gap-1 text-xs text-primary underline underline-offset-4'
          >
            打开
            <ExternalLink className='size-3' />
          </a>
        )}
      </div>

      <div className='space-y-1.5 text-muted-foreground'>
        {resource.host && <ResourceValue label='地址' value={resource.host} />}
        {resource.port && <ResourceValue label='端口' value={resource.port} />}
        {resource.url && <ResourceValue label='链接' value={resource.url} />}
        {resource.username && (
          <ResourceValue label='用户' value={resource.username} />
        )}
        {resource.password && (
          <ResourceValue label='密码' value={resource.password} sensitive />
        )}
        {resource.note && <div className='pt-1'>{resource.note}</div>}
      </div>
    </div>
  )
}

function ResourceValue({
  label,
  value,
  sensitive,
}: {
  label: string
  value: string
  sensitive?: boolean
}) {
  return (
    <div className='flex items-center justify-between gap-3'>
      <div className='min-w-0'>
        <span>{label}：</span>
        <span
          className={cn(
            'break-all text-foreground',
            sensitive && 'font-mono text-xs'
          )}
        >
          {value}
        </span>
      </div>
      <Button
        type='button'
        variant='ghost'
        size='icon'
        className='size-7 shrink-0'
        onClick={() => copyValue(value, label)}
      >
        <Copy className='size-3.5' />
      </Button>
    </div>
  )
}

function WikiEditor({
  draft,
  setDraft,
  pageID,
  isCreating,
  isSaving,
  onCancel,
  onSave,
}: {
  draft: DraftWikiPage
  setDraft: Dispatch<SetStateAction<DraftWikiPage>>
  pageID: string
  isCreating: boolean
  isSaving: boolean
  onCancel: () => void
  onSave: () => void
}) {
  const [collapsedResources, setCollapsedResources] = useState<Set<string>>(
    () => new Set()
  )
  const [isUploadingImage, setIsUploadingImage] = useState(false)
  const [tableRows, setTableRows] = useState(3)
  const [tableColumns, setTableColumns] = useState(3)
  const [isTablePopoverOpen, setIsTablePopoverOpen] = useState(false)
  const contentTextareaRef = useRef<HTMLTextAreaElement>(null)
  const imageInputRef = useRef<HTMLInputElement>(null)

  function updateDraft(value: Partial<DraftWikiPage>) {
    setDraft((current) => ({ ...current, ...value }))
  }

  function insertMarkdownSnippet(snippet: string) {
    setDraft((current) => {
      const textarea = contentTextareaRef.current
      if (!textarea) {
        return {
          ...current,
          content_md: current.content_md
            ? `${current.content_md}\n\n${snippet}`
            : snippet,
        }
      }

      const start = textarea.selectionStart ?? current.content_md.length
      const end = textarea.selectionEnd ?? start
      const before = current.content_md.slice(0, start)
      const after = current.content_md.slice(end)
      const prefix = before && !before.endsWith('\n') ? '\n\n' : ''
      const suffix = after && !after.startsWith('\n') ? '\n\n' : ''
      const nextContent = `${before}${prefix}${snippet}${suffix}${after}`
      const nextCursor = before.length + prefix.length + snippet.length

      window.requestAnimationFrame(() => {
        textarea.focus()
        textarea.setSelectionRange(nextCursor, nextCursor)
      })

      return {
        ...current,
        content_md: nextContent,
      }
    })
  }

  async function uploadImage(file: File) {
    if (!file.type.startsWith('image/')) {
      toast.error('请选择图片文件')
      return
    }

    setIsUploadingImage(true)
    try {
      const attachment = await uploadWikiAttachment(
        file,
        isCreating ? undefined : pageID
      )
      insertMarkdownSnippet(attachment.markdown)
      toast.success('图片已插入')
    } catch (error) {
      toast.error(imageUploadErrorMessage(error))
    } finally {
      setIsUploadingImage(false)
    }
  }

  function insertConfiguredTable() {
    insertMarkdownSnippet(tableTemplate(tableRows, tableColumns))
    setIsTablePopoverOpen(false)
  }

  function handleImageInputChange(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (file) {
      void uploadImage(file)
    }
  }

  function handleContentPaste(event: ClipboardEvent<HTMLTextAreaElement>) {
    const image = Array.from(event.clipboardData.files).find((file) =>
      file.type.startsWith('image/')
    )
    if (!image) return

    event.preventDefault()
    void uploadImage(image)
  }

  function addResource(resourceType: WikiResourceType) {
    setDraft((current) => ({
      ...current,
      resources: [...current.resources, emptyResource(resourceType)],
    }))
  }

  function toggleResourceCollapse(draftID: string) {
    setCollapsedResources((current) => {
      const next = new Set(current)
      if (next.has(draftID)) {
        next.delete(draftID)
      } else {
        next.add(draftID)
      }
      return next
    })
  }

  function applyRunbookTemplate() {
    setDraft((current) => ({
      ...current,
      page_type: 'runbook',
      category: '操作手册',
      content_md: current.content_md.trim()
        ? `${current.content_md}\n\n${runbookTemplate}`
        : runbookTemplate,
    }))
  }

  function applyTroubleshootingTemplate() {
    setDraft((current) => ({
      ...current,
      page_type: 'troubleshooting',
      category: '故障排查',
      content_md: current.content_md.trim()
        ? `${current.content_md}\n\n${troubleshootingTemplate}`
        : troubleshootingTemplate,
    }))
  }

  return (
    <div className='flex h-full min-h-0 flex-col'>
      <div className='flex items-center justify-between gap-3 border-b p-5'>
        <div>
          <h3 className='text-lg font-medium'>
            {isCreating ? '新建资料' : '编辑资料'}
          </h3>
        </div>
        <div className='flex gap-2'>
          <Button type='button' variant='outline' onClick={onCancel}>
            取消
          </Button>
          <Button type='button' disabled={isSaving} onClick={onSave}>
            <Save className='me-2 size-4' />
            保存
          </Button>
        </div>
      </div>

      <div className='min-h-0 flex-1 overflow-y-auto p-5'>
        <div className='grid gap-4 md:grid-cols-2'>
          <div className='space-y-2 md:col-span-2'>
            <label className='text-sm font-medium'>标题</label>
            <Input
              value={draft.title}
              onChange={(event) => updateDraft({ title: event.target.value })}
              placeholder='例如 生产数据库连接信息'
            />
          </div>

          <div className='space-y-2'>
            <label className='text-sm font-medium'>类型</label>
            <Select
              value={draft.page_type}
              onValueChange={(value) =>
                updateDraft({ page_type: value as WikiPageType })
              }
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {pageTypeOptions.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className='space-y-2'>
            <label className='text-sm font-medium'>分类</label>
            <Input
              value={draft.category}
              onChange={(event) =>
                updateDraft({ category: event.target.value })
              }
              list='wiki-category-options'
            />
            <datalist id='wiki-category-options'>
              {categoryOptions.map((item) => (
                <option key={item} value={item} />
              ))}
            </datalist>
          </div>

          <div className='space-y-2 md:col-span-2'>
            <label className='text-sm font-medium'>摘要</label>
            <Input
              value={draft.summary}
              onChange={(event) => updateDraft({ summary: event.target.value })}
              placeholder='一句话说明这条资料的用途'
            />
          </div>

          <div className='space-y-2 md:col-span-2'>
            <label className='text-sm font-medium'>标签</label>
            <Input
              value={draft.tags}
              onChange={(event) => updateDraft({ tags: event.target.value })}
              placeholder='例如 prod,postgres,backup'
            />
          </div>

          <div className='space-y-3 rounded-md border p-3 md:col-span-2'>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <div>
                <div className='text-sm font-medium'>快捷添加</div>
                <div className='text-xs text-muted-foreground'>
                  需要机器、链接或账号密码时再添加资源块。
                </div>
              </div>
              <div className='flex flex-wrap gap-2'>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => addResource('machine')}
                >
                  <Monitor className='size-4' />
                  机器信息
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => addResource('link')}
                >
                  <Link2 className='size-4' />
                  链接
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => addResource('credential')}
                >
                  <KeyRound className='size-4' />
                  账号密码
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => addResource('note')}
                >
                  <FileText className='size-4' />
                  备注
                </Button>
              </div>
            </div>

            {draft.resources.length > 0 && (
              <div className='space-y-3'>
                {draft.resources.map((resource, index) => (
                  <ResourceEditorCard
                    key={resource.draft_id}
                    index={index}
                    resourceCount={draft.resources.length}
                    resource={resource}
                    collapsed={collapsedResources.has(resource.draft_id)}
                    setDraft={setDraft}
                    onToggleCollapse={() =>
                      toggleResourceCollapse(resource.draft_id)
                    }
                  />
                ))}
              </div>
            )}
          </div>

          <div className='space-y-2 md:col-span-2'>
            <div className='sticky top-0 z-20 -mx-5 border-y bg-background/95 px-5 py-3 backdrop-blur supports-[backdrop-filter]:bg-background/80'>
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <label className='text-sm font-medium'>正文 Markdown</label>
                <div className='flex flex-wrap gap-2'>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    disabled={isUploadingImage}
                    onClick={() => imageInputRef.current?.click()}
                  >
                    <ImagePlus className='size-4' />
                    图片
                  </Button>
                  <Popover
                    open={isTablePopoverOpen}
                    onOpenChange={setIsTablePopoverOpen}
                  >
                    <PopoverTrigger asChild>
                      <Button type='button' variant='outline' size='sm'>
                        <Table2 className='size-4' />
                        表格
                      </Button>
                    </PopoverTrigger>
                    <PopoverContent align='end' className='w-72'>
                      <div className='space-y-4'>
                        <div>
                          <div className='text-sm font-medium'>插入表格</div>
                          <div className='text-xs text-muted-foreground'>
                            选择正文表格的数据行数和列数。
                          </div>
                        </div>
                        <div className='grid grid-cols-2 gap-3'>
                          <div className='space-y-2'>
                            <label className='text-xs font-medium'>行数</label>
                            <Input
                              type='number'
                              min={1}
                              max={20}
                              value={tableRows}
                              onChange={(event) =>
                                setTableRows(Number(event.target.value) || 1)
                              }
                            />
                          </div>
                          <div className='space-y-2'>
                            <label className='text-xs font-medium'>列数</label>
                            <Input
                              type='number'
                              min={1}
                              max={10}
                              value={tableColumns}
                              onChange={(event) =>
                                setTableColumns(Number(event.target.value) || 1)
                              }
                            />
                          </div>
                        </div>
                        <Button
                          type='button'
                          className='w-full'
                          onClick={insertConfiguredTable}
                        >
                          插入表格
                        </Button>
                      </div>
                    </PopoverContent>
                  </Popover>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={applyRunbookTemplate}
                  >
                    <BookOpen className='size-4' />
                    手册模板
                  </Button>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={applyTroubleshootingTemplate}
                  >
                    <Wrench className='size-4' />
                    故障模板
                  </Button>
                  <input
                    ref={imageInputRef}
                    type='file'
                    accept='image/png,image/jpeg,image/gif,image/webp'
                    className='hidden'
                    onChange={handleImageInputChange}
                  />
                </div>
              </div>
            </div>
            <Textarea
              ref={contentTextareaRef}
              value={draft.content_md}
              onChange={(event) =>
                updateDraft({ content_md: event.target.value })
              }
              onPaste={handleContentPaste}
              className='min-h-[320px] font-mono text-sm'
              placeholder='# 标题'
            />
          </div>
        </div>
      </div>
    </div>
  )
}

function ResourceEditorCard({
  index,
  resourceCount,
  resource,
  collapsed,
  setDraft,
  onToggleCollapse,
}: {
  index: number
  resourceCount: number
  resource: DraftWikiResource
  collapsed: boolean
  setDraft: Dispatch<SetStateAction<DraftWikiPage>>
  onToggleCollapse: () => void
}) {
  const Icon = resourceTypeIcon(resource.resource_type)
  const summary = [
    resource.title,
    resource.host,
    resource.url,
    resource.username,
  ]
    .filter(Boolean)
    .join(' / ')

  function updateResource(value: Partial<DraftWikiResource>) {
    setDraft((current) => ({
      ...current,
      resources: current.resources.map((item) =>
        item.draft_id === resource.draft_id ? { ...item, ...value } : item
      ),
    }))
  }

  function moveResource(offset: number) {
    setDraft((current) => {
      const currentIndex = current.resources.findIndex(
        (item) => item.draft_id === resource.draft_id
      )
      const targetIndex = currentIndex + offset
      if (
        currentIndex < 0 ||
        targetIndex < 0 ||
        targetIndex >= current.resources.length
      ) {
        return current
      }

      const resources = [...current.resources]
      const [moved] = resources.splice(currentIndex, 1)
      resources.splice(targetIndex, 0, moved)
      return { ...current, resources }
    })
  }

  function moveResourceTo(draggedDraftID: string) {
    setDraft((current) => {
      const fromIndex = current.resources.findIndex(
        (item) => item.draft_id === draggedDraftID
      )
      const targetIndex = current.resources.findIndex(
        (item) => item.draft_id === resource.draft_id
      )
      if (fromIndex < 0 || targetIndex < 0 || fromIndex === targetIndex) {
        return current
      }

      const resources = [...current.resources]
      const [moved] = resources.splice(fromIndex, 1)
      resources.splice(targetIndex, 0, moved)
      return { ...current, resources }
    })
  }

  function duplicateResource() {
    setDraft((current) => {
      const currentIndex = current.resources.findIndex(
        (item) => item.draft_id === resource.draft_id
      )
      if (currentIndex < 0) {
        return current
      }

      const source = current.resources[currentIndex]
      const resources = [...current.resources]
      resources.splice(currentIndex + 1, 0, {
        ...source,
        id: undefined,
        draft_id: draftID(),
        title: source.title ? `${source.title} 副本` : '',
      })
      return { ...current, resources }
    })
  }

  function removeResource() {
    setDraft((current) => ({
      ...current,
      resources: current.resources.filter(
        (item) => item.draft_id !== resource.draft_id
      ),
    }))
  }

  function handleDragStart(event: DragEvent<HTMLDivElement>) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', resource.draft_id)
  }

  function handleDragOver(event: DragEvent<HTMLDivElement>) {
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
  }

  function handleDrop(event: DragEvent<HTMLDivElement>) {
    event.preventDefault()
    moveResourceTo(event.dataTransfer.getData('text/plain'))
  }

  return (
    <div
      className='rounded-md border bg-muted/20 p-3'
      onDragOver={handleDragOver}
      onDrop={handleDrop}
    >
      <div
        className={cn(
          'flex items-center justify-between gap-2',
          !collapsed && 'mb-3'
        )}
      >
        <div className='flex min-w-0 items-center gap-2'>
          <div
            className='flex size-8 shrink-0 cursor-grab items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground active:cursor-grabbing'
            draggable
            onDragStart={handleDragStart}
            title='拖拽排序'
          >
            <GripVertical className='size-4' />
          </div>
          <Icon className='size-4 shrink-0' />
          <div className='min-w-0'>
            <div className='truncate text-sm font-medium'>
              {resourceTypeLabel(resource.resource_type)} {index + 1}
            </div>
            {collapsed && summary && (
              <div className='truncate text-xs text-muted-foreground'>
                {summary}
              </div>
            )}
          </div>
        </div>
        <div className='flex shrink-0 items-center gap-1'>
          <Button
            type='button'
            variant='ghost'
            size='icon'
            className='size-8'
            onClick={() => moveResource(-1)}
            disabled={index === 0}
            aria-label='上移资源'
            title='上移'
          >
            <ArrowUp className='size-4' />
          </Button>
          <Button
            type='button'
            variant='ghost'
            size='icon'
            className='size-8'
            onClick={() => moveResource(1)}
            disabled={index === resourceCount - 1}
            aria-label='下移资源'
            title='下移'
          >
            <ArrowDown className='size-4' />
          </Button>
          <Button
            type='button'
            variant='ghost'
            size='icon'
            className='size-8'
            onClick={duplicateResource}
            aria-label='复制资源'
            title='复制'
          >
            <Copy className='size-4' />
          </Button>
          <Button
            type='button'
            variant='ghost'
            size='icon'
            className='size-8'
            onClick={onToggleCollapse}
            aria-label={collapsed ? '展开资源' : '折叠资源'}
            title={collapsed ? '展开' : '折叠'}
          >
            {collapsed ? (
              <ChevronDown className='size-4' />
            ) : (
              <ChevronUp className='size-4' />
            )}
          </Button>
          <Button
            type='button'
            variant='ghost'
            size='icon'
            className='size-8'
            onClick={removeResource}
            aria-label='删除资源'
            title='删除'
          >
            <X className='size-4' />
          </Button>
        </div>
      </div>

      {!collapsed && (
        <div className='grid gap-3 md:grid-cols-2'>
          <div className='space-y-2'>
            <label className='text-sm font-medium'>资源类型</label>
            <Select
              value={resource.resource_type}
              onValueChange={(value) =>
                updateResource({ resource_type: value as WikiResourceType })
              }
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {resourceTypeOptions.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className='space-y-2'>
            <label className='text-sm font-medium'>名称</label>
            <Input
              value={resource.title}
              onChange={(event) =>
                updateResource({ title: event.target.value })
              }
              placeholder='例如 主库 / Grafana / 管理后台'
            />
          </div>

          {resource.resource_type === 'machine' && (
            <>
              <div className='space-y-2'>
                <label className='text-sm font-medium'>机器地址</label>
                <Input
                  value={resource.host}
                  onChange={(event) =>
                    updateResource({ host: event.target.value })
                  }
                  placeholder='例如 10.0.0.12'
                />
              </div>
              <div className='space-y-2'>
                <label className='text-sm font-medium'>端口</label>
                <Input
                  value={resource.port}
                  onChange={(event) =>
                    updateResource({ port: event.target.value })
                  }
                  placeholder='22'
                />
              </div>
            </>
          )}

          {resource.resource_type === 'link' && (
            <div className='space-y-2 md:col-span-2'>
              <label className='text-sm font-medium'>链接地址</label>
              <Input
                value={resource.url}
                onChange={(event) =>
                  updateResource({ url: event.target.value })
                }
                placeholder='https://'
              />
            </div>
          )}

          {resource.resource_type !== 'note' && (
            <>
              <div className='space-y-2'>
                <label className='text-sm font-medium'>登录用户</label>
                <Input
                  value={resource.username}
                  onChange={(event) =>
                    updateResource({ username: event.target.value })
                  }
                  placeholder='root / admin'
                />
              </div>
              <div className='space-y-2'>
                <label className='text-sm font-medium'>登录密码</label>
                <Input
                  value={resource.password}
                  onChange={(event) =>
                    updateResource({ password: event.target.value })
                  }
                  placeholder='直接显示给内部成员'
                />
              </div>
            </>
          )}

          <div className='space-y-2 md:col-span-2'>
            <label className='text-sm font-medium'>备注</label>
            <Textarea
              value={resource.note}
              onChange={(event) => updateResource({ note: event.target.value })}
              className='min-h-20'
              placeholder='补充说明、使用场景、注意事项'
            />
          </div>
        </div>
      )}
    </div>
  )
}
