import { test, expect } from '@playwright/test'

const suffix = Math.random().toString(36).slice(2, 7).toUpperCase()
const ccCode = `E2E${suffix}`
const ccName = `Test CeCo ${suffix}`
const ccNameUpdated = `Test CeCo ${suffix} Editado`

test.describe('Centros de Costo', () => {
  test('navegar a /cost-centers muestra tabla', async ({ page }) => {
    await page.goto('/cost-centers')
    await expect(page.getByRole('heading', { name: 'Centros de Costo' })).toBeVisible()
    await expect(page.locator('table')).toBeVisible()
  })

  test('crear CeCo con código único → aparece en tabla', async ({ page }) => {
    await page.goto('/cost-centers')

    // Open create dialog
    await page.getByRole('button', { name: 'Nuevo CeCo' }).click()

    // Fill form
    await expect(page.locator('#cc-code')).toBeVisible()
    await page.locator('#cc-code').fill(ccCode)
    await page.locator('#cc-name').fill(ccName)

    // Submit
    await page.getByRole('button', { name: 'Crear centro de costo' }).click()

    // Dialog should close
    await expect(page.locator('#cc-code')).not.toBeVisible({ timeout: 5_000 })

    // Search for the created CeCo
    await page.getByRole('textbox', { name: 'Buscar centros de costo' }).fill(ccCode)

    // Should appear in table
    await expect(page.getByText(ccCode, { exact: true })).toBeVisible({ timeout: 10_000 })
    await expect(page.getByText(ccName, { exact: true })).toBeVisible()
  })

  test('editar nombre → cambio persistido', async ({ page }) => {
    await page.goto('/cost-centers')

    // Search for the CeCo
    await page.getByRole('textbox', { name: 'Buscar centros de costo' }).fill(ccCode)
    await expect(page.getByText(ccName, { exact: true })).toBeVisible({ timeout: 10_000 })

    // Click edit button
    await page.getByRole('button', { name: `Editar ${ccName}` }).click()

    // Edit dialog should open
    await expect(page.locator('#edit-cc-name')).toBeVisible()

    // Clear and fill new name
    await page.locator('#edit-cc-name').clear()
    await page.locator('#edit-cc-name').fill(ccNameUpdated)

    // Submit
    await page.getByRole('button', { name: 'Guardar cambios' }).click()

    // Dialog should close
    await expect(page.locator('#edit-cc-name')).not.toBeVisible({ timeout: 5_000 })

    // Verify updated name appears
    await page.getByRole('textbox', { name: 'Buscar centros de costo' }).fill(ccCode)
    await expect(page.getByText(ccNameUpdated, { exact: true })).toBeVisible({ timeout: 10_000 })
  })
})
