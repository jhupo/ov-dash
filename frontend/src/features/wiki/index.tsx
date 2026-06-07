import {
  type Dispatch,
  type SetStateAction,
  useEffect,
  useMemo,
  useState,
} from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  BookOpen,
  ExternalLink,
  FileText,
  Link2,
  Monitor,
  Pencil,
  Plus,
  Save,
  SearchIcon,
  Trash2,
  Wrench,
} from 'lucide-react'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { MarkdownViewer } from '@/features/help-center/markdown-viewer'
import {
  createWikiPage,
  deleteWikiPage,
  getWikiPages,
  updateWikiPage,
  type SaveWikiPagePayload,
  type WikiPage,
  type WikiPageType,
} from '@/services/wiki'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
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

type DraftWikiPage = SaveWikiPagePayload

const pageTypeOptions: { label: string; value: WikiPageType; icon: typeof FileText }[] =
  [
    { label: '文档', value: 'document', icon: FileText },
    { label: '机器', value: 'machine', icon: Monitor },
    { label: '链接', value: 'link', icon: Link2 },
    { label: '手册', value: 'runbook', icon: BookOpen },
    { label: '故障', value: 'troubleshooting', icon: Wrench },
  ]

const categoryOptions = ['机器信息', '操作手册', '故障排查', '外部链接', '账号资料']

function emptyDraft(): DraftWikiPage {
  return {
    parent_id: '',
    title: '',
    page_type: 'document',
    category: '机器信息',
    summary: '',
    content_md: '',
    link_label: '',
    link_url: '',
    machine_host: '',
    machine_port: '',
    machine_username: '',
    tags: '',
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
    link_label: page.link_label,
    link_url: page.link_url,
    machine_host: page.machine_host,
    machine_port: page.machine_port,
    machine_username: page.machine_username,
    tags: page.tags,
  }
}

function pageTypeLabel(value: WikiPageType) {
  return pageTypeOptions.find((item) => item.value === value)?.label ?? '文档'
}

function pageTypeIcon(value: WikiPageType) {
  return pageTypeOptions.find((item) => item.value === value)?.icon ?? FileText
}

function normalizeDraft(draft: DraftWikiPage): SaveWikiPagePayload {
  return {
    parent_id: draft.parent_id?.trim() || '',
    title: draft.title.trim(),
    page_type: draft.page_type,
    category: draft.category.trim() || '资料库',
    summary: draft.summary.trim(),
    content_md: draft.content_md,
    link_label: draft.link_label.trim(),
    link_url: draft.link_url.trim(),
    machine_host: draft.machine_host.trim(),
    machine_port: draft.machine_port.trim(),
    machine_username: draft.machine_username.trim(),
    tags: draft.tags.trim(),
  }
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
        page.machine_host,
        page.link_url,
        page.tags,
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
    mutationFn: ({ id, payload }: { id: string; payload: SaveWikiPagePayload }) =>
      updateWikiPage(id, payload),
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
    setDraft({ ...emptyDraft(), page_type: pageType })
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
                              {page.summary || page.tags || page.machine_host}
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
        {(page.machine_host || page.link_url) && (
          <div className='mb-5 grid gap-3 md:grid-cols-2'>
            {page.machine_host && (
              <div className='rounded-md border p-3 text-sm'>
                <div className='mb-2 flex items-center gap-2 font-medium'>
                  <Monitor className='size-4' />
                  机器信息
                </div>
                <div className='grid gap-1 text-muted-foreground'>
                  <div>地址：{page.machine_host}</div>
                  {page.machine_port && <div>端口：{page.machine_port}</div>}
                  {page.machine_username && (
                    <div>用户：{page.machine_username}</div>
                  )}
                </div>
              </div>
            )}
            {page.link_url && (
              <div className='rounded-md border p-3 text-sm'>
                <div className='mb-2 flex items-center gap-2 font-medium'>
                  <Link2 className='size-4' />
                  关联链接
                </div>
                <a
                  className='inline-flex items-center gap-1 break-all text-primary underline underline-offset-4'
                  href={page.link_url}
                  target='_blank'
                  rel='noreferrer'
                >
                  {page.link_label || page.link_url}
                  <ExternalLink className='size-3.5 shrink-0' />
                </a>
              </div>
            )}
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

function WikiEditor({
  draft,
  setDraft,
  isCreating,
  isSaving,
  onCancel,
  onSave,
}: {
  draft: DraftWikiPage
  setDraft: Dispatch<SetStateAction<DraftWikiPage>>
  isCreating: boolean
  isSaving: boolean
  onCancel: () => void
  onSave: () => void
}) {
  function updateDraft(value: Partial<DraftWikiPage>) {
    setDraft((current) => ({ ...current, ...value }))
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

          <div className='space-y-2'>
            <label className='text-sm font-medium'>机器地址</label>
            <Input
              value={draft.machine_host}
              onChange={(event) =>
                updateDraft({ machine_host: event.target.value })
              }
              placeholder='例如 10.0.0.12'
            />
          </div>
          <div className='grid gap-4 sm:grid-cols-2'>
            <div className='space-y-2'>
              <label className='text-sm font-medium'>端口</label>
              <Input
                value={draft.machine_port}
                onChange={(event) =>
                  updateDraft({ machine_port: event.target.value })
                }
                placeholder='22'
              />
            </div>
            <div className='space-y-2'>
              <label className='text-sm font-medium'>用户</label>
              <Input
                value={draft.machine_username}
                onChange={(event) =>
                  updateDraft({ machine_username: event.target.value })
                }
                placeholder='root'
              />
            </div>
          </div>

          <div className='space-y-2'>
            <label className='text-sm font-medium'>链接名称</label>
            <Input
              value={draft.link_label}
              onChange={(event) =>
                updateDraft({ link_label: event.target.value })
              }
              placeholder='例如 Grafana 面板'
            />
          </div>
          <div className='space-y-2'>
            <label className='text-sm font-medium'>链接地址</label>
            <Input
              value={draft.link_url}
              onChange={(event) => updateDraft({ link_url: event.target.value })}
              placeholder='https://'
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

          <div className='space-y-2 md:col-span-2'>
            <label className='text-sm font-medium'>正文 Markdown</label>
            <Textarea
              value={draft.content_md}
              onChange={(event) =>
                updateDraft({ content_md: event.target.value })
              }
              className='min-h-[320px] font-mono text-sm'
              placeholder='# 标题'
            />
          </div>
        </div>
      </div>
    </div>
  )
}
