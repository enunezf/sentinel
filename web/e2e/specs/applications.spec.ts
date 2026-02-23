import { test, expect } from '@playwright/test'

const suffix = Math.random().toString(36).slice(2, 7)
const appName = `E2E App ${suffix}`
const appSlug = `e2e-app-${suffix}`

test.describe('Aplicaciones', () => {
  test('navegar a /applications muestra tabla', async ({ page }) => {
    await page.goto('/applications')
    await expect(page.getByRole('heading', { name: 'Aplicaciones' })).toBeVisible()
    await expect(page.locator('table')).toBeVisible()
  })

  test('crear aplicación → aparece en tabla', async ({ page }) => {
    await page.goto('/applications')

    // Open create dialog
    await page.getByRole('button', { name: 'Nueva aplicación' }).click()

    // Fill the form
    await expect(page.locator('#app-name')).toBeVisible()
    await page.locator('#app-name').fill(appName)

    // Slug is auto-generated; verify and clear to set manually
    await page.locator('#app-slug').fill(appSlug)

    // Submit
    await page.getByRole('button', { name: 'Crear aplicación' }).click()

    // Dialog should close
    await expect(page.locator('#app-name')).not.toBeVisible({ timeout: 5_000 })

    // Search for the created app
    await page.locator('input[placeholder="Buscar por nombre o slug..."]').fill(appName)

    // App should appear in table
    await expect(page.getByText(appName, { exact: true })).toBeVisible({ timeout: 10_000 })
  })

  test('ver detalle → 5 pestañas visibles', async ({ page }) => {
    await page.goto('/applications')

    // Search for the app
    await page.locator('input[placeholder="Buscar por nombre o slug..."]').fill(appName)
    await expect(page.getByText(appName, { exact: true })).toBeVisible({ timeout: 10_000 })

    // Click the "Ver detalle" button for the app
    await page
      .getByRole('button', { name: `Ver detalle de ${appName}` })
      .click()

    // Should navigate to detail page
    await page.waitForURL(/\/applications\/[a-z0-9-]+/, { timeout: 10_000 })

    // Verify all 5 tabs are present using role="tablist"
    const tablist = page.getByRole('tablist')
    await expect(tablist).toBeVisible()
    await expect(page.getByRole('tab', { name: 'Información General' })).toBeVisible()
    await expect(page.getByRole('tab', { name: 'Roles' })).toBeVisible()
    await expect(page.getByRole('tab', { name: 'Permisos' })).toBeVisible()
    await expect(page.getByRole('tab', { name: 'Centros de Costo' })).toBeVisible()
    await expect(page.getByRole('tab', { name: 'Usuarios' })).toBeVisible()
  })
})
