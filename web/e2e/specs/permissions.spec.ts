import { test, expect } from '@playwright/test'

const suffix = Math.random().toString(36).slice(2, 7)
// Permission code format: module.resource.action (lowercase, letters/numbers/_)
const permCode = `e2e.res${suffix}.read`

test.describe('Permisos', () => {
  test('navegar a /permissions muestra página con botón de creación', async ({ page }) => {
    await page.goto('/permissions')
    await expect(page.getByRole('heading', { name: 'Permisos' })).toBeVisible()
    // The "Nuevo permiso" button must always be present
    await expect(page.getByRole('button', { name: 'Nuevo permiso' })).toBeVisible()
  })

  test(`crear permiso → aparece en módulo "e2e"`, async ({ page }) => {
    await page.goto('/permissions')

    // Open create dialog
    await page.getByRole('button', { name: 'Nuevo permiso' }).click()

    // Fill the code
    await expect(page.locator('#perm-code')).toBeVisible()
    await page.locator('#perm-code').fill(permCode)

    // Fill optional description
    await page.locator('#perm-description').fill('Permiso creado por test E2E')

    // Select scope "Acción"
    await page.getByRole('button', { name: 'Acción' }).click()

    // Submit
    await page.getByRole('button', { name: 'Crear permiso' }).click()

    // Dialog should close
    await expect(page.locator('#perm-code')).not.toBeVisible({ timeout: 5_000 })

    // The permission should appear in the "e2e" module accordion
    await expect(page.getByText(permCode, { exact: true })).toBeVisible({ timeout: 10_000 })

    // The "e2e" module header should be visible
    await expect(page.getByText('e2e').first()).toBeVisible()
  })
})
