import { test, expect } from '@playwright/test'

const suffix = Math.random().toString(36).slice(2, 7)
const roleName = `e2erole${suffix}`

test.describe('Roles', () => {
  test('navegar a /roles muestra tabla', async ({ page }) => {
    await page.goto('/roles')
    await expect(page.getByRole('heading', { name: 'Roles' })).toBeVisible()
    await expect(page.locator('table')).toBeVisible()
  })

  test('crear rol → aparece en tabla', async ({ page }) => {
    await page.goto('/roles')

    // Open create dialog
    await page.getByRole('button', { name: 'Nuevo rol' }).click()

    // Fill name
    await expect(page.locator('#role-name')).toBeVisible()
    await page.locator('#role-name').fill(roleName)

    // Optional: fill description
    await page.locator('#role-description').fill('Rol creado por test E2E')

    // Submit (no permissions selected — that's optional)
    await page.getByRole('button', { name: 'Crear rol' }).click()

    // Dialog should close
    await expect(page.locator('#role-name')).not.toBeVisible({ timeout: 5_000 })

    // Role should appear in the table
    await expect(page.getByText(roleName, { exact: true })).toBeVisible({ timeout: 10_000 })
  })

  test('ver detalle → pestañas Permisos / Usuarios / Auditoría', async ({ page }) => {
    await page.goto('/roles')

    // Find and click the detail button for the test role
    const row = page.locator('tr').filter({ hasText: roleName })
    await expect(row).toBeVisible({ timeout: 10_000 })

    await row.getByRole('button', { name: `Ver detalle de ${roleName}` }).click()

    // Should navigate to role detail
    await page.waitForURL(/\/roles\/[a-z0-9-]+/, { timeout: 10_000 })

    // Verify tabs present
    await expect(page.getByRole('tab', { name: 'Permisos' })).toBeVisible()
    await expect(page.getByRole('tab', { name: 'Usuarios' })).toBeVisible()
    await expect(page.getByRole('tab', { name: 'Auditoría' })).toBeVisible()
  })
})
