export const HELP_DOCUMENT_STORAGE_KEY = 'ov-dash.help-center.markdown'

export const DEFAULT_HELP_DOCUMENT = `# 帮助中心

欢迎使用 OV Dash。这里可以维护操作说明、图片和流程图。

## 常用流程

\`\`\`mermaid
flowchart TD
  A[打开仪表盘] --> B[查看服务状态]
  B --> C[配置代理]
  C --> D[保存设置]
\`\`\`

## 图片

使用 Markdown 图片语法插入图片：

![OV Dash](https://dummyimage.com/960x360/111827/ffffff&text=OV+Dash)

## 列表

- 支持标题、段落和列表
- 支持链接和图片
- 支持代码块和简单流程图
`

export function readHelpDocument() {
  if (typeof window === 'undefined') return DEFAULT_HELP_DOCUMENT

  return (
    window.localStorage.getItem(HELP_DOCUMENT_STORAGE_KEY) ||
    DEFAULT_HELP_DOCUMENT
  )
}

export function saveHelpDocument(markdown: string) {
  window.localStorage.setItem(HELP_DOCUMENT_STORAGE_KEY, markdown)
}

export function resetHelpDocument() {
  window.localStorage.removeItem(HELP_DOCUMENT_STORAGE_KEY)
}
