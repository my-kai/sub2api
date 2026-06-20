const CALLBACK_DOMAIN_TOKEN_SPLIT_RE = /[\s,，]+/
const CALLBACK_DOMAIN_PATTERN =
  /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$/
const CALLBACK_WILDCARD_PREFIX = '*.'

/**
 * Converts an admin-entered callback domain or URL to the canonical host token.
 * This mirrors the backend setting validation so the form can stay forgiving.
 */
export function normalizeCallbackAuthDomain(raw: string): string {
  const value = String(raw || '').trim().toLowerCase()
  if (!value) {
    return ''
  }

  if (value.startsWith(CALLBACK_WILDCARD_PREFIX)) {
    const base = value.slice(CALLBACK_WILDCARD_PREFIX.length)
    return `${CALLBACK_WILDCARD_PREFIX}${stripDomainNoise(base)}`
  }

  if (value.includes('://')) {
    try {
      return normalizeCallbackAuthHost(new URL(value).host)
    } catch {
      return ''
    }
  }

  if (/[/?#]/.test(value)) {
    return ''
  }
  return normalizeCallbackAuthHost(value)
}

export function normalizeCallbackAuthDomains(items: string[] | null | undefined): string[] {
  if (!items || items.length === 0) {
    return []
  }
  const seen = new Set<string>()
  const normalized: string[] = []
  for (const item of items) {
    const domain = normalizeCallbackAuthDomain(item)
    if (!isCallbackAuthDomainValid(domain) || seen.has(domain)) {
      continue
    }
    seen.add(domain)
    normalized.push(domain)
  }
  return normalized
}

export function parseCallbackAuthDomainInput(input: string): string[] {
  if (!input || !input.trim()) {
    return []
  }
  const seen = new Set<string>()
  const normalized: string[] = []
  for (const token of input.split(CALLBACK_DOMAIN_TOKEN_SPLIT_RE)) {
    const domain = normalizeCallbackAuthDomain(token)
    if (!isCallbackAuthDomainValid(domain) || seen.has(domain)) {
      continue
    }
    seen.add(domain)
    normalized.push(domain)
  }
  return normalized
}

export function isCallbackAuthDomainValid(domain: string): boolean {
  if (!domain) {
    return false
  }
  if (domain.startsWith(CALLBACK_WILDCARD_PREFIX)) {
    return CALLBACK_DOMAIN_PATTERN.test(domain.slice(CALLBACK_WILDCARD_PREFIX.length))
  }
  return domain === 'localhost' || isIPv4(domain) || CALLBACK_DOMAIN_PATTERN.test(domain)
}

function normalizeCallbackAuthHost(raw: string): string {
  let host = String(raw || '').trim().toLowerCase()
  if (!host) {
    return ''
  }
  if (host.startsWith('[') && host.includes(']')) {
    host = host.slice(1, host.indexOf(']'))
  } else if (host.includes(':')) {
    const [candidate] = host.split(':')
    host = candidate
  }
  return stripDomainNoise(host)
}

function stripDomainNoise(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/\.$/, '')
    .replace(/[^a-z0-9.-]/g, '')
}

function isIPv4(value: string): boolean {
  const parts = value.split('.')
  return parts.length === 4 && parts.every((part) => {
    if (!/^\d{1,3}$/.test(part)) return false
    const n = Number(part)
    return n >= 0 && n <= 255
  })
}
