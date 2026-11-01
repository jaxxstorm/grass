import { describe, it, expect, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import App from './App.vue'
import { safeURL } from './types'

describe('archive presentation', () => {
  it('rejects executable and invalid source links', () => {
    expect(safeURL('javascript:alert(1)')).toBeUndefined()
    expect(safeURL('data:text/html,bad')).toBeUndefined()
    expect(safeURL('https://example.test/post')).toBe('https://example.test/post')
  })
  it('shows legacy coverage and removes URL filters', async () => {
    vi.stubGlobal('matchMedia', () => ({ matches: false }))
    const payload = { Snapshot: 'one', Results: [], Total: 0, ArchiveTotal: 1, Page: 1, PageSize: 25, Keywords: [], Platforms: [], Terms: [], Authors: [], Activity: [], Coverage: { KnownAuthors: 0, UnknownAuthors: 0, WithContent: 0, WithoutContent: 0 } }
    vi.stubGlobal('fetch', vi.fn(async (url: string) => new Response(JSON.stringify(url.includes('/status') ? { Source: 'sqlite', State: 'ready', Snapshot: 'one', LoadedAt: '2026-01-01T00:00:00Z' } : payload), { status: 200 })))
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<div />' } }] })
    await router.push('/?keyword=grass')
    const wrapper = mount(App, { global: { plugins: [router] } })
    await flushPromises()
    expect(wrapper.text()).toContain('No conversations match')
    expect(wrapper.text()).toContain('Author data is unavailable')
    await wrapper.get('.chip').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.keyword).toBeUndefined()
    wrapper.unmount()
    vi.unstubAllGlobals()
  })
})
