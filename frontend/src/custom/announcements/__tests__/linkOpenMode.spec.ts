import { describe, expect, it } from 'vitest'
import {
  ANNOUNCEMENT_LINK_OPEN_MODE_CURRENT,
  ANNOUNCEMENT_LINK_OPEN_MODE_NEW_TAB,
  buildAnnouncementLinkMarkdown,
  findEditableAnnouncementLink,
  replaceAnnouncementLink,
} from '../linkOpenMode'

describe('announcement link open mode', () => {
  it('builds standard Markdown for current-page links', () => {
    expect(buildAnnouncementLinkMarkdown({
      text: '文档',
      href: '/docs',
      openMode: ANNOUNCEMENT_LINK_OPEN_MODE_CURRENT,
    })).toBe('[文档](/docs)')
  })

  it('builds HTML anchor for new-window links', () => {
    expect(buildAnnouncementLinkMarkdown({
      text: '文档',
      href: 'https://example.test?a=1&b=2',
      openMode: ANNOUNCEMENT_LINK_OPEN_MODE_NEW_TAB,
    })).toBe('<a href="https://example.test?a=1&amp;b=2" target="_blank" rel="noopener noreferrer">文档</a>')
  })

  it('uses the URL as link text when text is blank', () => {
    expect(buildAnnouncementLinkMarkdown({
      text: '   ',
      href: '/docs',
      openMode: ANNOUNCEMENT_LINK_OPEN_MODE_CURRENT,
    })).toBe('[/docs](/docs)')

    expect(buildAnnouncementLinkMarkdown({
      text: '',
      href: 'https://example.test?a=1&b=2',
      openMode: ANNOUNCEMENT_LINK_OPEN_MODE_NEW_TAB,
    })).toBe('<a href="https://example.test?a=1&amp;b=2" target="_blank" rel="noopener noreferrer">https://example.test?a=1&amp;b=2</a>')
  })

  it('replaces an existing Markdown link with configured open mode', () => {
    const source = '查看 [文档](/docs) 后继续。'
    const link = findEditableAnnouncementLink(source, { href: '/docs', text: '文档' })

    expect(link).not.toBeNull()
    expect(replaceAnnouncementLink(source, link!, {
      text: '帮助文档',
      href: '/help',
      openMode: ANNOUNCEMENT_LINK_OPEN_MODE_NEW_TAB,
    })).toBe('查看 <a href="/help" target="_blank" rel="noopener noreferrer">帮助文档</a> 后继续。')
  })

  it('matches generated HTML links after browser attribute decoding', () => {
    const source = '查看 <a href="https://example.test?a=1&amp;b=2" target="_blank" rel="noopener noreferrer">文档</a>。'
    const link = findEditableAnnouncementLink(source, {
      href: 'https://example.test?a=1&b=2',
      text: '文档',
    })

    expect(link?.openMode).toBe(ANNOUNCEMENT_LINK_OPEN_MODE_NEW_TAB)
  })

  it('can choose a repeated link by occurrence', () => {
    const source = '[文档](/docs) 和 [文档](/docs)'
    const second = findEditableAnnouncementLink(source, {
      href: '/docs',
      text: '文档',
      occurrence: 1,
    })

    expect(second?.start).toBe(14)
  })
})
