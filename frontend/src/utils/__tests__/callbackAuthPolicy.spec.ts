import { describe, expect, it } from 'vitest'
import {
  isCallbackAuthDomainValid,
  normalizeCallbackAuthDomain,
  normalizeCallbackAuthDomains,
  parseCallbackAuthDomainInput,
} from '../callbackAuthPolicy'

describe('callbackAuthPolicy', () => {
  it('normalizes domains, wildcard domains and URLs', () => {
    expect(normalizeCallbackAuthDomain(' https://Example.COM:8443/cb ')).toBe('example.com')
    expect(normalizeCallbackAuthDomain('*.Sub.Example.COM')).toBe('*.sub.example.com')
    expect(normalizeCallbackAuthDomain('localhost:3000')).toBe('localhost')
  })

  it('deduplicates normalized domains', () => {
    expect(normalizeCallbackAuthDomains(['example.com', 'https://example.com/cb', '*.example.com'])).toEqual([
      'example.com',
      '*.example.com',
    ])
  })

  it('parses pasted input and drops invalid tokens', () => {
    expect(parseCallbackAuthDomainInput('example.com, bad/path *.example.com')).toEqual([
      'example.com',
      '*.example.com',
    ])
  })

  it('validates exact and wildcard domains', () => {
    expect(isCallbackAuthDomainValid('example.com')).toBe(true)
    expect(isCallbackAuthDomainValid('*.example.com')).toBe(true)
    expect(isCallbackAuthDomainValid('localhost')).toBe(true)
    expect(isCallbackAuthDomainValid('bad_domain')).toBe(false)
  })
})
