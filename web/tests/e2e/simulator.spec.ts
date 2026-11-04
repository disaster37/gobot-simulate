import { expect, test } from '@playwright/test'

test('dashboard renders boards and opens a board workbench', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Gobot Simulator' })).toBeVisible()
  const boardLinks = page.getByRole('link', { name: 'Open board' })
  if ((await boardLinks.count()) === 0) {
    test.skip(true, 'the demo simulator is not running with boards')
  }
  await boardLinks.first().click()
  await expect(page.getByText('Pin matrix')).toBeVisible()
})
