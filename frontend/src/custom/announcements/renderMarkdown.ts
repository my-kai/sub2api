import { marked } from 'marked'
import DOMPurify from 'dompurify'

marked.setOptions({
  breaks: true,
  gfm: true,
})

/**
 * Render announcement Markdown into sanitized HTML.
 *
 * @param content - Announcement Markdown content saved by admins.
 * @returns Sanitized HTML that preserves explicit link open-mode attributes.
 */
export function renderAnnouncementMarkdown(content: string | null | undefined): string {
  if (!content) {
    return ''
  }

  const html = marked.parse(content) as string
  const sanitized = DOMPurify.sanitize(html, {
    ADD_ATTR: ['target', 'rel'],
  })

  return normalizeAnnouncementLinks(sanitized)
}

function normalizeAnnouncementLinks(html: string): string {
  const template = document.createElement('template')
  template.innerHTML = html

  // 只保留公告编辑器支持的 target 取值，避免管理员手写 HTML 时带入不可预期窗口行为。
  template.content.querySelectorAll('a').forEach((link) => {
    const target = link.getAttribute('target')
    if (target === '_blank') {
      link.setAttribute('rel', 'noopener noreferrer')
      return
    }
    if (target && target !== '_self') {
      link.removeAttribute('target')
    }
    if (target !== '_blank') {
      link.removeAttribute('rel')
    }
  })

  return template.innerHTML
}
