import { Fragment } from 'react'
import { cn } from '@/lib/utils'

type MarkdownViewerProps = {
  markdown: string
}

type FlowEdge = {
  from: string
  to: string
}

type FlowNode = {
  id: string
  label: string
}

function escapeHtml(value: string) {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function inlineMarkdown(value: string) {
  const escaped = escapeHtml(value)

  return escaped
    .replace(
      /!\[([^\]]*)\]\(([^)]+)\)/g,
      '<img src="$2" alt="$1" class="my-4 max-h-[420px] w-full rounded-md border object-contain" />'
    )
    .replace(
      /\[([^\]]+)\]\(([^)]+)\)/g,
      '<a href="$2" target="_blank" rel="noreferrer" class="font-medium text-primary underline underline-offset-4">$1</a>'
    )
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/`([^`]+)`/g, '<code class="rounded bg-muted px-1 py-0.5">$1</code>')
}

function getFlowNode(raw: string): FlowNode {
  const match = raw.trim().match(/^([A-Za-z0-9_-]+)(?:\[(.+)\])?$/)
  if (!match) return { id: raw.trim(), label: raw.trim() }

  return {
    id: match[1],
    label: match[2] ?? match[1],
  }
}

function parseFlowchart(code: string) {
  const nodes = new Map<string, FlowNode>()
  const edges: FlowEdge[] = []

  code
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line && !line.startsWith('flowchart'))
    .forEach((line) => {
      const [fromRaw, toRaw] = line.split(/-->|---|==>/)
      if (!fromRaw || !toRaw) return

      const from = getFlowNode(fromRaw)
      const to = getFlowNode(toRaw)
      nodes.set(from.id, from)
      nodes.set(to.id, to)
      edges.push({ from: from.id, to: to.id })
    })

  return { nodes: [...nodes.values()], edges }
}

function Flowchart({ code }: { code: string }) {
  const { nodes, edges } = parseFlowchart(code)

  if (!nodes.length) {
    return (
      <pre className='overflow-x-auto rounded-md border bg-muted p-4 text-sm'>
        <code>{code}</code>
      </pre>
    )
  }

  return (
    <div className='my-5 rounded-md border bg-card p-4'>
      <div className='flex flex-wrap items-center gap-3'>
        {nodes.map((node, index) => (
          <Fragment key={node.id}>
            {index > 0 && (
              <span className='text-muted-foreground'>{'->'}</span>
            )}
            <div className='rounded-md border bg-background px-3 py-2 text-sm font-medium shadow-xs'>
              {node.label}
            </div>
          </Fragment>
        ))}
      </div>
      {edges.length > 0 && (
        <div className='mt-4 space-y-1 text-xs text-muted-foreground'>
          {edges.map((edge) => {
            const from = nodes.find((node) => node.id === edge.from)
            const to = nodes.find((node) => node.id === edge.to)
            return (
              <div key={`${edge.from}-${edge.to}`}>
                {from?.label ?? edge.from} {'->'} {to?.label ?? edge.to}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

function renderBlock(block: string, index: number) {
  const trimmed = block.trim()
  if (!trimmed) return null

  if (trimmed.startsWith('```')) {
    const lines = trimmed.split('\n')
    const language = lines[0].replace('```', '').trim()
    const lastLine = lines[lines.length - 1]
    const code = lines
      .slice(1, lastLine?.startsWith('```') ? -1 : undefined)
      .join('\n')

    if (language === 'mermaid') {
      return <Flowchart key={index} code={code} />
    }

    return (
      <pre
        key={index}
        className='overflow-x-auto rounded-md border bg-muted p-4 text-sm'
      >
        <code>{code}</code>
      </pre>
    )
  }

  if (trimmed.startsWith('# ')) {
    return (
      <h1 key={index} className='text-3xl font-bold tracking-tight'>
        {trimmed.slice(2)}
      </h1>
    )
  }

  if (trimmed.startsWith('## ')) {
    return (
      <h2 key={index} className='pt-2 text-2xl font-semibold tracking-tight'>
        {trimmed.slice(3)}
      </h2>
    )
  }

  if (trimmed.startsWith('### ')) {
    return (
      <h3 key={index} className='pt-1 text-xl font-semibold'>
        {trimmed.slice(4)}
      </h3>
    )
  }

  if (/^[-*] /m.test(trimmed)) {
    return (
      <ul key={index} className='ml-5 list-disc space-y-2'>
        {trimmed.split('\n').map((line) => (
          <li
            key={line}
            dangerouslySetInnerHTML={{
              __html: inlineMarkdown(line.replace(/^[-*] /, '')),
            }}
          />
        ))}
      </ul>
    )
  }

  if (/^\d+\. /m.test(trimmed)) {
    return (
      <ol key={index} className='ml-5 list-decimal space-y-2'>
        {trimmed.split('\n').map((line) => (
          <li
            key={line}
            dangerouslySetInnerHTML={{
              __html: inlineMarkdown(line.replace(/^\d+\. /, '')),
            }}
          />
        ))}
      </ol>
    )
  }

  return (
    <p
      key={index}
      className='leading-7 text-foreground/90'
      dangerouslySetInnerHTML={{ __html: inlineMarkdown(trimmed) }}
    />
  )
}

function splitBlocks(markdown: string) {
  const blocks: string[] = []
  let codeBlock: string[] | null = null

  for (const line of markdown.split('\n')) {
    if (line.startsWith('```')) {
      if (codeBlock) {
        codeBlock.push(line)
        blocks.push(codeBlock.join('\n'))
        codeBlock = null
      } else {
        codeBlock = [line]
      }
      continue
    }

    if (codeBlock) {
      codeBlock.push(line)
      continue
    }

    const last = blocks[blocks.length - 1]
    if (!line.trim()) {
      if (last?.trim()) blocks.push('')
      continue
    }

    if (!last || !last.trim()) {
      blocks.push(line)
      continue
    }

    blocks[blocks.length - 1] = `${last}\n${line}`
  }

  if (codeBlock) blocks.push(codeBlock.join('\n'))

  return blocks.filter((block) => block.trim())
}

export function MarkdownViewer({ markdown }: MarkdownViewerProps) {
  return (
    <div
      className={cn(
        'mx-auto flex w-full max-w-4xl flex-col gap-5',
        'text-sm md:text-base'
      )}
    >
      {splitBlocks(markdown).map(renderBlock)}
    </div>
  )
}
