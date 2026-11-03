import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, NetworkError, api, isTodayTask, taskKey, taskOrigin } from './api'

afterEach(() => vi.unstubAllGlobals())

function respond(status: number, body?: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(body === undefined ? null : JSON.stringify(body), { status })),
  )
}

describe('api errors', () => {
  it('carries the status and code of an error body', async () => {
    respond(502, { error: 'service: atlassian rejected the credentials', code: 'atlassian_unauthorized' })
    const err = await api.syncAtlassian().catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 502, code: 'atlassian_unauthorized' })
  })

  it('treats a failed fetch as the server being down', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => Promise.reject(new TypeError('Failed to fetch'))))
    await expect(api.healthz()).rejects.toBeInstanceOf(NetworkError)
  })

  it('treats a bare gateway error from a dev proxy as the server being down', async () => {
    respond(502)
    await expect(api.listTasks()).rejects.toBeInstanceOf(NetworkError)
  })

  it('returns nothing for 204', async () => {
    respond(204)
    await expect(api.deleteTodo(1)).resolves.toBeUndefined()
  })
})

describe('task IDs', () => {
  it('splits the origin from the key', () => {
    expect(taskKey('local-WHF-003')).toBe('WHF-003')
    expect(taskKey('jira-SEC-142')).toBe('SEC-142')
    expect(taskOrigin('jira-SEC-142')).toBe('jira')
    expect(taskOrigin('SEC-142')).toBeUndefined()
  })
})

describe('isTodayTask', () => {
  it('lists open tasks with or without todos for today', () => {
    expect(isTodayTask({ status: 'todo' }, false)).toBe(true)
    expect(isTodayTask({ status: 'in-progress' }, false)).toBe(true)
    expect(isTodayTask({ status: 'in-progress' }, true)).toBe(true)
  })

  it('lists done and canceled tasks only while they have todos for today', () => {
    expect(isTodayTask({ status: 'done' }, false)).toBe(false)
    expect(isTodayTask({ status: 'canceled' }, false)).toBe(false)
    expect(isTodayTask({ status: 'done' }, true)).toBe(true)
    expect(isTodayTask({ status: 'canceled' }, true)).toBe(true)
  })
})
