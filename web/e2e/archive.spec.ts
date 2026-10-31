import { expect, test } from '@playwright/test'

test.beforeEach(async ({ page }) => {
  await page.route('**/api/status', route => route.fulfill({ json: { Source: 'sqlite', State: 'ready', Snapshot: 'one', LoadedAt: '2026-01-01T00:00:00Z', Refreshing: false } }))
  await page.route('**/api/archive?*', async route => {
    const query = new URL(route.request().url()).searchParams
    if (query.get('q') === 'older') await new Promise(resolve => setTimeout(resolve, 500))
    const title = query.get('q') || 'A conversation about networking'
    await route.fulfill({ json: { Snapshot: 'one', LoadedAt: '2026-01-01T00:00:00Z', Total: 1, ArchiveTotal: 1, Page: 1, PageSize: 25,
      Results: [{ ID: 'one', Platform: 'Reddit', Keyword: 'grass', Title: title, URL: 'https://example.test/post', Timestamp: 1767225600, Content: '', AuthorKey: '', AuthorName: '', AuthorHandle: '' }],
      Keywords: [{ Key: 'grass', Label: 'grass', Count: 1 }], Platforms: [], Authors: query.get('q') === 'rich' ? [{ Key: 'alice-key', Label: 'Alice', Platform: 'Reddit', Count: 1 }] : [], Terms: [{ Key: 'networking', Label: 'networking', Count: 1 }], Activity: [{ Key: '2026-01-01', Label: '2026-01-01', Count: 1 }],
      Coverage: { KnownAuthors: 0, UnknownAuthors: 1, WithContent: 0, WithoutContent: 1 } } })
  })
})

test('linked filters, legacy coverage, details, and mobile layout', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByText('1 results with unknown authors')).toBeVisible()
  await page.screenshot({ path: test.info().outputPath('archive.png'), fullPage: true })
  await page.locator('.rank-row').focus()
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(/keyword=grass/)
  await page.getByRole('button', { name: 'networking: 1 results' }).click()
  await expect(page).toHaveURL(/term=networking/)
  await page.getByRole('button', { name: 'A conversation about networking' }).click()
  await expect(page.getByText('This record contains a title only.')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.locator('dialog')).not.toBeVisible()
  const fits = await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
  expect(fits).toBe(true)
  await page.getByRole('button', { name: 'Use dark theme' }).click()
  await expect(page.locator('html')).toHaveClass(/dark/)
  await page.screenshot({ path: test.info().outputPath('archive-dark.png'), fullPage: true })
  await page.getByRole('button', { name: 'Clear all', exact: true }).click()
  await expect(page).not.toHaveURL(/term=/)
})

test('snapshot conflicts restart pagination', async ({ page }) => {
  let replace = false
  await page.route('**/api/archive?*', async route => {
    if (replace && new URL(route.request().url()).searchParams.has('snapshot')) {
      replace = false
      await route.fulfill({ status: 409, json: { code: 'snapshot_changed', message: 'Snapshot changed' } })
    } else await route.fallback()
  })
  await page.goto('/?page=2')
  await expect(page.getByRole('button', { name: 'A conversation about networking' })).toBeVisible()
  replace = true
  await page.route('**/api/status', route => route.fulfill({ json: { Source: 'sqlite', State: 'ready', Snapshot: 'two', LoadedAt: '2026-01-01T01:00:00Z' } }))
  await expect(page).not.toHaveURL(/page=2/, { timeout: 6000 })
  await expect(page.getByRole('button', { name: 'A conversation about networking' })).toBeVisible()
})

test('new searches win and navigation restores filters', async ({ page }) => {
  await page.goto('/?keyword=grass')
  const input = page.getByRole('searchbox')
  await input.fill('older')
  await page.waitForURL(/q=older/)
  await input.fill('newer')
  await expect(page.getByRole('button', { name: 'newer', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'older', exact: true })).not.toBeVisible()
  await page.goBack()
  await expect(input).toHaveValue('older')
})

test('refresh errors keep the archive visible', async ({ page }) => {
  await page.route('**/api/archive/refresh', route => route.fulfill({ status: 429, json: { code: 'refresh_cooldown', message: 'Wait one minute between archive loads.' } }))
  await page.goto('/')
  await expect(page.getByRole('button', { name: 'A conversation about networking' })).toBeVisible()
  await page.getByRole('button', { name: 'Refresh archive' }).click()
  await expect(page.getByRole('alert')).toContainText('Wait one minute')
  await expect(page.getByRole('button', { name: 'A conversation about networking' })).toBeVisible()
})

test('author drill-down is keyboard accessible', async ({ page }) => {
  await page.goto('/?q=rich')
  const author = page.locator('.author-row')
  await author.focus()
  await expect(author).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(/author=alice-key/)
  await expect(page.getByText('author: Selected author')).toBeVisible()
})

test('initial load errors stay visible during refresh cooldown', async ({ page }) => {
  await page.route('**/api/status', route => route.fulfill({ json: {
    Source: 'dynamodb', State: 'error', Snapshot: '', Refreshing: false,
    Error: 'DynamoDB reads were throttled. See server logs for details.',
    RetryAt: new Date(Date.now() + 60000).toISOString(),
  } }))
  await page.route('**/api/archive?*', route => route.fulfill({ status: 503, json: { code: 'archive_load_failed', message: 'DynamoDB reads were throttled. See server logs for details.' } }))
  await page.goto('/')
  await expect(page.getByRole('alert')).toContainText('DynamoDB reads were throttled')
  await expect(page.getByRole('button', { name: /Refresh available in/ })).toBeDisabled()
  await expect(page.getByRole('button', { name: /Retry in/ })).toBeDisabled()
  await expect(page.getByText('Archive load failed — see server logs')).toBeVisible()
  await expect(page.getByText('Preparing your archive')).not.toBeVisible()
})

test('cached results stay searchable during synchronization and reconciliation is explicit', async ({ page }) => {
  let reconciled = false
  await page.route('**/api/status', route => route.fulfill({ json: { Source: 'dynamodb', State: 'ready', Snapshot: 'one', LoadedAt: '2026-01-01T00:00:00Z', LastSync: '2026-01-01T00:00:00Z', IndexPath: '/cache/grass/archive.db', SyncMode: 'journal', Phase: 'catchup', Examined: 12, Target: 20, Checkpoint: 12, Refreshing: false } }))
  await page.route('**/api/archive/reconcile', route => { reconciled = true; return route.fulfill({ status: 202, json: {} }) })
  await page.goto('/')
  await expect(page.getByRole('button', { name: 'A conversation about networking' })).toBeVisible()
  await expect(page.getByText('Search complete words.', { exact: false })).toBeVisible()
  await page.getByText('Synchronization options').click()
  await expect(page.getByText('Reconciliation reads the complete source', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: 'Reconcile archive', exact: true }).click()
  expect(reconciled).toBe(true)
  await page.route('**/api/status', route => route.fulfill({ json: { Source: 'dynamodb', State: 'ready', Snapshot: 'one', LoadedAt: '2026-01-01T00:00:00Z', IndexPath: '/cache/grass/archive.db', SyncMode: 'journal', Phase: 'catchup', Examined: 12, Refreshing: true } }))
  await expect(page.getByText('Searching cached data while synchronizing', { exact: false })).toBeVisible()
  await expect(page.getByRole('button', { name: 'A conversation about networking' })).toBeVisible()
})
