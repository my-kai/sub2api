export const ANNOUNCEMENT_LINK_OPEN_MODE_CURRENT = 'current'
export const ANNOUNCEMENT_LINK_OPEN_MODE_NEW_TAB = 'new_tab'

export type AnnouncementLinkOpenMode =
  | typeof ANNOUNCEMENT_LINK_OPEN_MODE_CURRENT
  | typeof ANNOUNCEMENT_LINK_OPEN_MODE_NEW_TAB

export interface EditableAnnouncementLink {
  start: number
  end: number
  text: string
  href: string
  openMode: AnnouncementLinkOpenMode
}

export interface AnnouncementLinkInput {
  text: string
  href: string
  openMode: AnnouncementLinkOpenMode
}

interface LinkSearchHint {
  href: string
  text?: string
  occurrence?: number
}

const markdownLinkPattern = /(!?)\[((?:\\.|[^\]\\\n])*)\]\(\s*(?:<([^>\n]+)>|([^)\s\n]+))(?:\s+(['"])(.*?)\5)?\s*\)/g
const htmlAnchorPattern = /<a\b([^>]*)>([\s\S]*?)<\/a>/gi

/**
 * Build the Markdown source persisted in announcement content.
 *
 * @param input - Link text, URL and the selected open mode.
 * @returns Markdown-compatible link source. New-window links use raw HTML because standard Markdown has no target attribute.
 */
export function buildAnnouncementLinkMarkdown(input: AnnouncementLinkInput): string {
  const href = input.href.trim()
  const text = input.text.trim() || href

  if (input.openMode === ANNOUNCEMENT_LINK_OPEN_MODE_NEW_TAB) {
    return `<a href="${escapeHtmlAttribute(href)}" target="_blank" rel="noopener noreferrer">${escapeHtmlText(text)}</a>`
  }

  return `[${escapeMarkdownLinkText(text)}](${escapeMarkdownLinkDestination(href)})`
}

/**
 * Replace a previously located announcement link without touching the rest of the Markdown body.
 *
 * @param source - Current announcement Markdown content.
 * @param current - Link range returned by findEditableAnnouncementLink.
 * @param next - Link values to persist.
 * @returns Updated announcement Markdown content.
 */
export function replaceAnnouncementLink(
  source: string,
  current: EditableAnnouncementLink,
  next: AnnouncementLinkInput,
): string {
  const replacement = buildAnnouncementLinkMarkdown(next)
  return `${source.slice(0, current.start)}${replacement}${source.slice(current.end)}`
}

/**
 * Find the source range for a clicked preview link.
 *
 * @param source - Current announcement Markdown content.
 * @param hint - Link href and optional visible text from the preview DOM.
 * @returns The first matching Markdown or HTML link, if it can be mapped back to source.
 */
export function findEditableAnnouncementLink(
  source: string,
  hint: LinkSearchHint,
): EditableAnnouncementLink | null {
  const links = parseEditableLinks(source)
  const href = normalizeComparable(hint.href)
  const text = normalizeComparable(hint.text ?? '')
  const occurrence = Math.max(0, hint.occurrence ?? 0)

  const exact = links.filter((link) => {
    if (normalizeComparable(link.href) !== href) {
      return false
    }
    return text === '' || normalizeComparable(link.text) === text
  })
  if (exact.length > 0) {
    return exact[occurrence] ?? exact[0]
  }

  const hrefOnly = links.filter((link) => normalizeComparable(link.href) === href)
  return hrefOnly[occurrence] ?? hrefOnly[0] ?? null
}

/**
 * Parse editable links from both standard Markdown links and raw HTML anchors.
 *
 * @param source - Markdown source.
 * @returns Source ranges and editable values sorted by source position.
 */
export function parseEditableLinks(source: string): EditableAnnouncementLink[] {
  const links: EditableAnnouncementLink[] = []

  markdownLinkPattern.lastIndex = 0
  let markdownMatch: RegExpExecArray | null
  while ((markdownMatch = markdownLinkPattern.exec(source)) !== null) {
    if (markdownMatch[1] === '!') {
      continue
    }
    links.push({
      start: markdownMatch.index,
      end: markdownMatch.index + markdownMatch[0].length,
      text: unescapeMarkdownLinkText(markdownMatch[2]),
      href: markdownMatch[3] ?? markdownMatch[4] ?? '',
      openMode: ANNOUNCEMENT_LINK_OPEN_MODE_CURRENT,
    })
  }

  htmlAnchorPattern.lastIndex = 0
  let htmlMatch: RegExpExecArray | null
  while ((htmlMatch = htmlAnchorPattern.exec(source)) !== null) {
    const attrs = htmlMatch[1] ?? ''
    const href = readHtmlAttribute(attrs, 'href')
    if (!href) {
      continue
    }

    links.push({
      start: htmlMatch.index,
      end: htmlMatch.index + htmlMatch[0].length,
      text: htmlToPlainText(htmlMatch[2] ?? ''),
      href,
      openMode: readHtmlAttribute(attrs, 'target') === '_blank'
        ? ANNOUNCEMENT_LINK_OPEN_MODE_NEW_TAB
        : ANNOUNCEMENT_LINK_OPEN_MODE_CURRENT,
    })
  }

  return links.sort((a, b) => a.start - b.start)
}

function normalizeComparable(value: string): string {
  return value.trim().replace(/\s+/g, ' ')
}

function escapeHtmlText(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

function escapeHtmlAttribute(value: string): string {
  return escapeHtmlText(value).replace(/"/g, '&quot;')
}

function escapeMarkdownLinkText(value: string): string {
  return value
    .replace(/\\/g, '\\\\')
    .replace(/\[/g, '\\[')
    .replace(/\]/g, '\\]')
}

function escapeMarkdownLinkDestination(value: string): string {
  if (/[\s)]/.test(value)) {
    return `<${value.replace(/>/g, '%3E')}>`
  }
  return value
}

function unescapeMarkdownLinkText(value: string): string {
  return value.replace(/\\([\\[\]])/g, '$1')
}

function readHtmlAttribute(attrs: string, name: string): string | null {
  const pattern = new RegExp(`\\b${name}\\s*=\\s*(?:"([^"]*)"|'([^']*)'|([^\\s"'>]+))`, 'i')
  const match = pattern.exec(attrs)
  const value = match?.[1] ?? match?.[2] ?? match?.[3] ?? null
  return value === null ? null : decodeBasicEntities(value)
}

function htmlToPlainText(value: string): string {
  return decodeBasicEntities(value.replace(/<[^>]*>/g, ''))
}

function decodeBasicEntities(value: string): string {
  return value
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&')
}
