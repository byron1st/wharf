import { describe, expect, it } from 'vitest'
import { jiraURL } from './jira'

describe('jiraURL', () => {
  it('builds the browse link and trims trailing slashes', () => {
    expect(jiraURL('https://example.atlassian.net//', 'CIS-12345')).toBe('https://example.atlassian.net/browse/CIS-12345')
  })

  it('encodes the key', () => {
    expect(jiraURL('https://example.atlassian.net', 'A B')).toBe('https://example.atlassian.net/browse/A%20B')
  })

  it('is null without a site URL', () => {
    expect(jiraURL(undefined, 'CIS-1')).toBeNull()
    expect(jiraURL('', 'CIS-1')).toBeNull()
  })
})
