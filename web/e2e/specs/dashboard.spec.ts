import { test, expect } from '@playwright/test'

test.describe('Dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/dashboard')
  })

  test('título "Dashboard" visible', async ({ page }) => {
    await expect(page.getByRole('heading', { name: 'Dashboard', level: 1 })).toBeVisible()
  })

  test('4 tarjetas de métricas presentes', async ({ page }) => {
    // Each metric card is a Link with the stat label
    await expect(page.getByText('Usuarios').first()).toBeVisible()
    await expect(page.getByText('Aplicaciones').first()).toBeVisible()
    await expect(page.getByText('Roles').first()).toBeVisible()
    await expect(page.getByText('Permisos').first()).toBeVisible()
  })

  test('tabla de actividad reciente visible', async ({ page }) => {
    await expect(page.getByRole('heading', { name: 'Actividad reciente' })).toBeVisible()
    // The activity table shows event/actor/time columns
    await expect(page.getByText('Evento', { exact: true })).toBeVisible()
    await expect(page.getByText('Actor', { exact: true })).toBeVisible()
  })
})
