import { expect, test } from '@playwright/test'

test('dashboard renders boards and opens a board workbench', async ({ page }) => {
  const pageErrors: string[] = []
  page.on('pageerror', (error) => pageErrors.push(error.message))
  page.on('console', (message) => {
    if (message.type() === 'error') {
      pageErrors.push(message.text())
    }
  })

  const response = await page.goto('/simulator')
  expect(response?.status()).toBe(200)
  await expect(page.getByRole('link', { name: 'Gobot Simulator' })).toBeVisible()
  expect(pageErrors, `page errors: ${pageErrors.join('; ')}`).toEqual([])

  const boardLinks = page.getByRole('link', { name: 'Open board' })
  if ((await boardLinks.count()) === 0) {
    test.skip(true, 'the demo simulator is not running with boards')
  }
  await boardLinks.first().click()
  await expect(page.getByText('Pin matrix')).toBeVisible()
})

test('pressing a button is visible on the I/O timeline', async ({ page }) => {
  await page.goto('/simulator')
  const boardLinks = page.getByRole('link', { name: 'Open board' })
  if ((await boardLinks.count()) === 0) {
    test.skip(true, 'the demo simulator is not running with boards')
  }
  await boardLinks.first().click()

  const press = page.getByRole('button', { name: 'Hold / Push' }).first()
  if ((await press.count()) === 0) {
    test.skip(true, 'the selected board has no pressable inputs')
  }
  await press.click()
  await page.waitForTimeout(150)

  await page.getByRole('link', { name: 'Timeline' }).click()
  await expect(page.getByText('pin-write').first()).toBeVisible()
})
