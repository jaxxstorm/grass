<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { ArchiveResponse, RecordItem, Status } from './types'
import { safeURL } from './types'

const route = useRoute(), router = useRouter()
const data = ref<ArchiveResponse>(), status = ref<Status>(), error = ref(''), loading = ref(false)
const search = ref(''), selected = ref<RecordItem>(), drawer = ref<HTMLDialogElement>()
function storedTheme() { try { return window.localStorage.getItem('grass-theme') } catch { return null } }
const dark = ref(storedTheme() === 'dark' || (!storedTheme() && matchMedia('(prefers-color-scheme: dark)').matches))
watch(dark, value => { document.documentElement.classList.toggle('dark', value); try { window.localStorage.setItem('grass-theme', value ? 'dark' : 'light') } catch { /* Theme still works when storage is unavailable. */ } }, { immediate: true })
const chips = computed(() => Object.entries(route.query).flatMap(([key, value]) => key === 'page' ? [] : (Array.isArray(value) ? value : [value]).filter(Boolean).map(value => ({ key, value: String(value) }))))
const maxActivity = computed(() => Math.max(1, ...(data.value?.Activity.map(x => x.Count) || [])))
const platforms = ['Reddit', 'HackerNews', 'Bluesky', 'Fediverse', 'YouTube', 'X']
const now = ref(Date.now())
const retrySeconds = computed(() => Math.max(0, Math.ceil((Date.parse(status.value?.RetryAt || '') - now.value) / 1000)) || 0)
const refreshDisabled = computed(() => status.value?.Refreshing || retrySeconds.value > 0)
let controller: AbortController | undefined, debounce: ReturnType<typeof setTimeout> | undefined, poll: ReturnType<typeof setInterval> | undefined
let generation = 0

function filter(key: string, value: string) {
  const query = { ...route.query, page: undefined, [key]: value || undefined }
  void router.push({ query }).then(failure => { if (failure) void load() })
}
function remove(key: string, value: string) {
  const existing = route.query[key]
  if (Array.isArray(existing)) void router.push({ query: { ...route.query, page: undefined, [key]: existing.filter(x => x !== value) } })
  else filter(key, '')
}
function inputSearch() { clearTimeout(debounce); controller?.abort(); generation++; debounce = setTimeout(() => filter('q', search.value), 300) }
function dateFilter(key: string, event: Event) {
  const value = (event.target as HTMLInputElement).value
  filter(key, value ? `${value}T00:00:00Z` : '')
}
function recorded(timestamp: number) { return new Date(timestamp * 1000).toLocaleString() }
async function load() {
  controller?.abort(); controller = new AbortController()
  const current = ++generation
  loading.value = true; error.value = ''
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(route.query)) for (const item of Array.isArray(value) ? value : [value]) if (item) params.append(key, item)
  if (data.value?.Snapshot) params.set('snapshot', data.value.Snapshot)
  try {
    const response = await fetch(`/api/archive?${params}`, { signal: controller.signal })
    const result = await response.json()
    if (current !== generation) return
    if (response.status === 409) {
      data.value = undefined
      if (route.query.page) await router.replace({ query: { ...route.query, page: undefined } })
      else await load()
      return
    }
    if (!response.ok) { if (result.code !== 'archive_loading') error.value = result.message; return }
    data.value = result
  } catch (err) { if (current === generation && (err as Error).name !== 'AbortError') error.value = 'Could not reach the archive. Try again.' }
  finally { if (current === generation) loading.value = false }
}
async function readStatus() {
  now.value = Date.now()
  try {
    const response = await fetch('/api/status'); if (!response.ok) throw new Error()
    const next: Status = await response.json()
    const changed = next.Snapshot && next.Snapshot !== status.value?.Snapshot
    status.value = next
    if (changed) await load()
  } catch { if (!data.value) error.value = 'Could not reach the archive. Try again.' }
}
async function refresh(reconcile = false) {
  error.value = ''
  try {
    const response = await fetch(reconcile ? '/api/archive/reconcile' : '/api/archive/refresh', { method: 'POST' })
    if (!response.ok) { const result = await response.json(); await readStatus(); error.value = result.message; return }
    await readStatus()
  } catch { error.value = 'Refresh failed. Try again.' }
}
async function open(item: RecordItem) { selected.value = item; await nextTick(); drawer.value?.showModal() }
function close() { drawer.value?.close(); selected.value = undefined }
watch(() => route.fullPath, () => { clearTimeout(debounce); search.value = String(route.query.q || ''); void load() }, { immediate: true })
onMounted(() => { void readStatus(); poll = setInterval(readStatus, 2000) })
onBeforeUnmount(() => { controller?.abort(); clearTimeout(debounce); clearInterval(poll) })
</script>

<template>
  <div class="app-shell">
    <header class="topbar">
      <a href="/" class="brand"><span class="brand-icon" aria-hidden="true">↟</span> grass <span class="brand-divider">/</span> <span class="brand-page">Archive</span></a>
      <div class="header-actions"><span class="source"><span class="status-dot"></span>{{ status?.Source || 'Connecting' }}</span><button class="icon-button" @click="dark = !dark" :aria-label="dark ? 'Use light theme' : 'Use dark theme'">{{ dark ? '☀' : '☾' }}</button></div>
    </header>

    <main>
      <section class="intro">
        <div><p class="eyebrow">YOUR COLLECTED CONVERSATIONS</p><h1>A little perspective.<br><span>A lot of signal.</span></h1><p class="subheading">Explore the ideas and people growing around your keywords.</p></div>
        <div class="freshness"><button class="button" @click="refresh()" :disabled="refreshDisabled">{{ status?.Refreshing ? 'Refreshing…' : retrySeconds ? `Refresh available in ${retrySeconds}s` : '↻ Refresh archive' }}</button><small>{{ status?.Snapshot ? `Local data updated ${new Date(status.LoadedAt).toLocaleString()}` : status?.State === 'error' ? 'Archive load failed — see server logs' : 'Preparing your local index' }}</small><small v-if="status?.LastSync && !status.LastSync.startsWith('0001')">Last synchronized {{ new Date(status.LastSync).toLocaleString() }}</small></div>
      </section>

      <section class="search-panel" aria-label="Archive filters">
        <label class="search-field"><span aria-hidden="true">⌕</span><input v-model="search" @input="inputSearch" type="search" maxlength="256" placeholder="Search collected posts, topics, or people…" aria-label="Search collected posts"><kbd>ARCHIVE SEARCH</kbd></label>
        <p class="search-help">Search complete words. All words must match; “network” does not match “networking”.</p>
        <div class="filter-row">
          <label>Platform<select :value="route.query.platform || ''" @change="filter('platform', ($event.target as HTMLSelectElement).value)"><option value="">All platforms</option><option v-for="platform in platforms" :key="platform">{{ platform }}</option></select></label>
          <label>Stored keyword<input :value="route.query.keyword || ''" @change="filter('keyword', ($event.target as HTMLInputElement).value)" placeholder="All keywords"></label>
          <label>Recorded from (UTC)<input type="date" :value="String(route.query.from || '').slice(0,10)" @change="dateFilter('from', $event)"></label>
          <label>Before (UTC)<input type="date" :value="String(route.query.to || '').slice(0,10)" @change="dateFilter('to', $event)"></label>
        </div>
        <div v-if="chips.length" class="chips"><button v-for="chip in chips" :key="`${chip.key}:${chip.value}`" class="chip" @click="remove(chip.key, chip.value)">{{ chip.key }}: {{ chip.key === 'author' ? 'Selected author' : chip.value }} <span aria-hidden="true">×</span><span class="sr-only">Remove filter</span></button><button class="text-button" @click="router.push({ query: {} })">Clear all</button></div>
      </section>

      <div v-if="error || status?.Error" role="alert" class="notice">{{ status?.Stale ? 'Showing the last successful snapshot. ' : '' }}{{ status?.Error || error }} <span v-if="status?.Error && error && error !== status.Error"> {{ error }}</span> <button class="text-button" :disabled="!!status?.Error && refreshDisabled" @click="status?.Error ? refresh() : load()">{{ status?.Error && retrySeconds ? `Retry in ${retrySeconds}s` : 'Retry' }}</button></div>
      <section v-if="status?.IndexPath" class="sync-status" aria-label="Local synchronization">
        <p v-if="status.Refreshing" role="status">{{ status.Snapshot ? 'Searching cached data while synchronizing' : 'Building the local search index' }} · {{ status.Phase }} · {{ (status.Examined || 0).toLocaleString() }} items examined<span v-if="status.Target"> · journal target {{ status.Target }}</span></p>
        <p v-else-if="status.SyncMode === 'legacy'">Cached archive ready. This source has no journal baseline; deploy the journal-capable collector, then reconcile to enable incremental synchronization.</p>
        <details><summary>Synchronization options</summary><p>Index: {{ status.IndexPath }}</p><p>Mode: {{ status.SyncMode || 'initial import' }} · Checkpoint: {{ status.Checkpoint || 0 }}</p><p>Reconciliation reads the complete source and replaces the local index when finished. Existing cached results remain available.</p><button class="button" @click="refresh(true)" :disabled="refreshDisabled">Reconcile archive</button></details>
      </section>
      <div v-if="!data && !error && status?.State !== 'error'" role="status" class="skeleton"><span></span><span></span><span></span><p>Gathering your archive… Large DynamoDB archives can take several minutes while reads are paced. See server logs for progress.</p></div>

      <template v-if="data">
        <section class="metrics" aria-label="Archive summary">
          <div class="metric"><p>Collected results</p><strong>{{ data.Total.toLocaleString() }}</strong><small>of {{ data.ArchiveTotal.toLocaleString() }} in this snapshot</small></div>
          <div class="metric"><p>Known author coverage</p><strong>{{ data.Total ? Math.round(data.Coverage.KnownAuthors / data.Total * 100) : 0 }}<em>%</em></strong><small>{{ data.Coverage.UnknownAuthors }} results with unknown authors</small></div>
          <div class="metric"><p>Content coverage</p><strong>{{ data.Total ? Math.round(data.Coverage.WithContent / data.Total * 100) : 0 }}<em>%</em></strong><small>{{ data.Coverage.WithoutContent }} results use titles only</small></div>
          <div class="metric activity"><p>Activity <small>· recorded timestamp, UTC</small></p><svg class="activity-chart" :viewBox="`0 0 ${Math.max(1,data.Activity.length)} 100`" preserveAspectRatio="none" role="img" aria-label="Daily collected results"><rect v-for="(day,index) in data.Activity" :key="day.Key" :x="index" :y="100-day.Count/maxActivity*100" width="0.8" :height="day.Count/maxActivity*100"><title>{{ day.Label }}: {{ day.Count }}</title></rect></svg><details><summary>Daily counts</summary><ul><li v-for="day in data.Activity" :key="day.Key">{{ day.Label }}: {{ day.Count }}</li></ul></details></div>
        </section>

        <div class="content-grid" :aria-busy="loading">
          <section class="results-panel">
            <div class="section-heading"><h2>Conversations <span class="count-pill">{{ data.Total }}</span></h2><small>{{ loading ? 'Updating…' : 'Newest recorded first' }}</small></div>
            <div v-if="!data.Total" class="empty"><span aria-hidden="true">↟</span><h3>{{ data.ArchiveTotal ? 'No conversations match just yet.' : 'Room to grow.' }}</h3><p>{{ data.ArchiveTotal ? 'Try a broader search or remove a filter.' : 'Collected results will appear here after your next collection run and archive refresh.' }}</p><button v-if="chips.length" class="button" @click="router.push({ query: {} })">Clear filters</button></div>
            <article v-for="item in data.Results" :key="item.ID" class="result-card">
              <div class="result-meta"><span class="platform-badge">{{ item.Platform }}</span><button v-if="item.Keyword" class="keyword-link" @click="filter('keyword', item.Keyword)">{{ item.Keyword }}</button><time :datetime="new Date(item.Timestamp * 1000).toISOString()">{{ recorded(item.Timestamp) }}</time></div>
              <button class="result-title" @click="open(item)">{{ item.Title || 'Untitled result' }}</button>
              <p v-if="item.Content" class="excerpt">{{ item.Content }}</p>
              <div class="result-footer"><button v-if="item.AuthorKey" class="author-link" @click="filter('author', item.AuthorKey)"><span class="avatar" aria-hidden="true">{{ (item.AuthorName || item.AuthorHandle || '?').slice(0,1) }}</span>{{ item.AuthorName || item.AuthorHandle || item.AuthorID }}</button><small v-else>Author unavailable in this record</small><a v-if="safeURL(item.URL)" :href="safeURL(item.URL)" target="_blank" rel="noopener noreferrer">View source ↗</a></div>
            </article>
            <nav v-if="data.Total" class="pagination" aria-label="Result pages"><button class="button" :disabled="data.Page <= 1 || loading" @click="router.push({ query: { ...route.query, page: String(data.Page - 1) } })">← Previous</button><span>Page {{ data.Page }} of {{ Math.max(1, Math.ceil(data.Total / data.PageSize)) }}</span><button class="button" :disabled="data.Page * data.PageSize >= data.Total || loading" @click="router.push({ query: { ...route.query, page: String(data.Page + 1) } })">Next →</button></nav>
          </section>

          <aside>
            <section class="panel"><div class="section-heading"><h2>Keywords</h2><small>Stored matches</small></div><p class="panel-caption">One stored keyword per collected result.</p><button v-for="word in data.Keywords" :key="word.Key" class="rank-row" @click="word.Key && filter('keyword', word.Key)" :disabled="!word.Key"><span>{{ word.Label || 'No stored keyword' }}</span><strong>{{ word.Count }}</strong><i :style="{ width: `${word.Count / Math.max(1,data.Total) * 100}%` }"></i></button><p v-if="!data.Keywords.length" class="muted">No keywords in this selection.</p></section>
            <section class="panel"><div class="section-heading"><h2>Growing topics</h2><small>Word cloud</small></div><p class="panel-caption">Terms in titles and available content. Each result counts once.</p><div class="word-cloud"><button v-for="word in data.Terms" :key="word.Key" @click="filter('term', word.Key)" :style="{ fontSize: `${13 + 21 * word.Count / Math.max(1, data.Terms[0]?.Count || 1)}px` }" :aria-label="`${word.Label}: ${word.Count} results`">{{ word.Label }}</button></div><details><summary>View term counts</summary><button v-for="word in data.Terms" :key="word.Key" class="list-row" @click="filter('term', word.Key)"><span>{{ word.Label }}</span><strong>{{ word.Count }}</strong></button></details><p v-if="!data.Terms.length" class="muted">No eligible terms in this selection.</p></section>
            <section class="panel"><div class="section-heading"><h2>Most active authors</h2></div><p class="panel-caption">Activity within this collected archive.</p><button v-for="(author,index) in data.Authors" :key="author.Key" class="author-row" @click="filter('author', author.Key)"><span class="rank">{{ index + 1 }}</span><span><b>{{ author.Label }}</b><small>{{ author.Platform }}</small></span><strong>{{ author.Count }}</strong></button><p v-if="!data.Authors.length" class="muted">Author data is unavailable for this selection. Older records may not include author identities.</p></section>
          </aside>
        </div>
      </template>
      <footer>Grass · A quieter view of the conversation.<span>Searches your archive, not live platforms.</span></footer>
    </main>
    <dialog ref="drawer" class="drawer" @close="selected = undefined" @click="($event.target === drawer) && close()">
      <div v-if="selected" class="drawer-body"><button class="button close" @click="close" autofocus>Close ×</button><p class="eyebrow">{{ selected.Platform }} · {{ selected.Keyword }}</p><h2>{{ selected.Title }}</h2><p class="muted">{{ selected.AuthorName || selected.AuthorHandle || 'Author unavailable' }} · Recorded {{ recorded(selected.Timestamp) }}</p><p class="detail-content">{{ selected.Content || 'This record contains a title only. Post content was not stored.' }}</p><a v-if="safeURL(selected.URL)" class="button" :href="safeURL(selected.URL)" target="_blank" rel="noopener noreferrer">Open original ↗</a></div>
    </dialog>
  </div>
</template>
