import { test, expect } from '@playwright/test'

// Generate a unique suffix per test run to avoid collisions
const suffix = Math.random().toString(36).slice(2, 7)
const username = `e2etest${suffix}`
const email = `e2etest${suffix}@sentinel.test`
const password = 'E2eTest@1234'

test.describe('Usuarios', () => {
  test('navegar a /users muestra tabla', async ({ page }) => {
    await page.goto('/users')
    await expect(page.getByRole('heading', { name: 'Usuarios' })).toBeVisible()
    // DataTable renders a table element
    await expect(page.locator('table')).toBeVisible()
  })

  test('crear usuario con username único → aparece en tabla', async ({ page }) => {
    await page.goto('/users')

    // Open create dialog
    await page.getByRole('button', { name: 'Nuevo usuario' }).click()

    // Fill the form
    await expect(page.locator('#new-username')).toBeVisible()
    await page.locator('#new-username').fill(username)
    await page.locator('#new-email').fill(email)
    await page.locator('#new-password').fill(password)

    // Submit
    await page.getByRole('button', { name: 'Crear usuario' }).click()

    // Dialog should close and success toast should appear
    await expect(page.locator('#new-username')).not.toBeVisible({ timeout: 5_000 })

    // Search for the created user
    await page.getByRole('textbox', { name: 'Buscar usuarios' }).fill(username)

    // User should appear in the table
    await expect(page.getByText(`@${username}`, { exact: true })).toBeVisible({ timeout: 10_000 })
  })

  test('toggle estado del usuario → badge cambia', async ({ page }) => {
    await page.goto('/users')

    // Search for the test user created in the previous test
    await page.getByRole('textbox', { name: 'Buscar usuarios' }).fill(username)

    // Wait for search results
    await expect(page.getByText(`@${username}`, { exact: true })).toBeVisible({ timeout: 10_000 })

    // User should be active by default
    const row = page.locator('tr').filter({ hasText: `@${username}` })
    await expect(row.getByText('Activo')).toBeVisible()

    // Click the actions dropdown for this user
    await row.getByRole('button', { name: `Acciones para @${username}` }).click()

    // Click "Desactivar"
    await page.getByText('Desactivar').click()

    // Confirm the action
    await page.getByRole('button', { name: 'Confirmar' }).click()

    // Wait for the table to reload and verify badge changed
    await page.getByRole('textbox', { name: 'Buscar usuarios' }).fill(username)
    await expect(page.getByText(`@${username}`, { exact: true })).toBeVisible({ timeout: 10_000 })
    const updatedRow = page.locator('tr').filter({ hasText: `@${username}` })
    await expect(updatedRow.getByText('Inactivo')).toBeVisible({ timeout: 8_000 })
  })
})
