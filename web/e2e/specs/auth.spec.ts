import { test, expect } from '@playwright/test'

// These tests run without pre-authentication
test.use({ storageState: { cookies: [], origins: [] } })

test.describe('Autenticación', () => {
  test('login válido redirige a /dashboard', async ({ page }) => {
    await page.goto('/login')

    await page.locator('#username').fill('admin')
    await page.locator('#password').fill('Admin@Local1!')
    await page.getByRole('button', { name: 'Iniciar sesión' }).click()

    await page.waitForURL('**/dashboard', { timeout: 15_000 })
    await expect(page).toHaveURL(/\/dashboard/)
  })

  test('contraseña incorrecta muestra alerta de error', async ({ page }) => {
    await page.goto('/login')

    await page.locator('#username').fill('admin')
    await page.locator('#password').fill('ContrasenaIncorrecta999!')
    await page.getByRole('button', { name: 'Iniciar sesión' }).click()

    // The error message uses 'contrasena' (no accent) per utils.ts
    await expect(page.getByRole('alert')).toContainText('Usuario o contrasena incorrectos', {
      timeout: 10_000,
    })
    // Should remain on the login page
    await expect(page).toHaveURL(/\/login/)
  })

  test('logout redirige a /login', async ({ page }) => {
    // Step 1: login first
    await page.goto('/login')
    await page.locator('#username').fill('admin')
    await page.locator('#password').fill('Admin@Local1!')
    await page.getByRole('button', { name: 'Iniciar sesión' }).click()
    await page.waitForURL('**/dashboard', { timeout: 15_000 })

    // Step 2: open user dropdown and click logout
    await page.getByRole('button', { name: 'Menú de usuario' }).click()
    await page.getByText('Cerrar sesión').click()

    // Step 3: verify redirect to login
    await page.waitForURL('**/login', { timeout: 10_000 })
    await expect(page).toHaveURL(/\/login/)
  })
})
