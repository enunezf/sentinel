import { test as setup, expect } from '@playwright/test'
import fs from 'fs'
import path from 'path'

const AUTH_FILE = 'e2e/.auth/admin.json'

// Bootstrap password set in deploy/local/.env (BOOTSTRAP_ADMIN_PASSWORD)
const PRIMARY_PASSWORD = 'Admin@Local1!'
// Password used after mandatory first-change (must_change_password: true)
const CHANGED_PASSWORD = 'Admin@Sentinel2!'

setup('authenticate as admin', async ({ page }) => {
  // Ensure the .auth directory exists
  const dir = path.dirname(AUTH_FILE)
  if (!fs.existsSync(dir)) {
    fs.mkdirSync(dir, { recursive: true })
  }

  // Try primary password first, then the post-change password as fallback
  for (const pwd of [PRIMARY_PASSWORD, CHANGED_PASSWORD]) {
    await page.goto('/login')

    await page.locator('#username').fill('admin')
    await page.locator('#password').fill(pwd)
    await page.getByRole('button', { name: 'Iniciar sesión' }).click()

    // Wait for either: redirect to dashboard, mandatory change modal, or login error
    const result = await Promise.race([
      page.waitForURL('**/dashboard', { timeout: 10_000 }).then(() => 'dashboard' as const),
      page
        .locator('text=Cambio de contraseña requerido')
        .waitFor({ timeout: 10_000 })
        .then(() => 'modal' as const),
    ]).catch(() => 'error' as const)

    if (result === 'dashboard') {
      break
    }

    if (result === 'modal') {
      // Admin is forced to change their password before continuing
      await page.locator('#current_password').fill(pwd)
      await page.locator('#new_password').fill(CHANGED_PASSWORD)
      await page.locator('#confirm_password').fill(CHANGED_PASSWORD)
      await page.getByRole('button', { name: 'Cambiar contraseña' }).click()
      await page.waitForURL('**/dashboard', { timeout: 15_000 })
      break
    }

    // 'error': this password failed — clear state and try the next one
  }

  await expect(page).toHaveURL(/\/dashboard/)

  // Save authenticated storage state (localStorage tokens + cookies)
  await page.context().storageState({ path: AUTH_FILE })
})
