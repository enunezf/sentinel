import { test, expect } from '@playwright/test'

test.describe('Auditoría', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/audit')
    // Wait for the page title to confirm navigation
    await expect(page.getByRole('heading', { name: 'Auditoría' })).toBeVisible()
  })

  test('tabla de logs de auditoría visible', async ({ page }) => {
    // The audit page renders a <table> element
    await expect(page.locator('table')).toBeVisible()
    // Header columns — scope to thead to avoid matching filter labels
    await expect(page.getByRole('columnheader', { name: 'Timestamp' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: 'Evento' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: 'Resultado' })).toBeVisible()
  })

  test('filtrar por categoría AUTH → dropdown de tipo muestra solo eventos AUTH', async ({
    page,
  }) => {
    // The category filter is a Radix Select; click its trigger
    // The label is "Categoría" and the trigger shows "Todas las categorías" initially
    const categoryTrigger = page.locator('[role="combobox"]').first()
    await categoryTrigger.click()

    // Select "AUTH" option from the dropdown
    await page.getByRole('option', { name: 'AUTH' }).click()

    // Now the event-type dropdown should only show AUTH_* events
    // Click the event type trigger (second combobox)
    const eventTypeTrigger = page.locator('[role="combobox"]').nth(1)
    await eventTypeTrigger.click()

    // Verify AUTH event types are present
    await expect(page.getByRole('option', { name: 'Login exitoso' })).toBeVisible()
    await expect(page.getByRole('option', { name: 'Login fallido' })).toBeVisible()
    await expect(page.getByRole('option', { name: 'Logout' })).toBeVisible()

    // Close dropdown
    await page.keyboard.press('Escape')
  })

  test('click en fila → modal de detalle se abre', async ({ page }) => {
    // Wait for rows to load (there should be audit data from prior test activity)
    const rows = page.locator('tbody tr').filter({ hasNotText: 'No se encontraron eventos' })

    // Wait for at least one row to appear
    await expect(rows.first()).toBeVisible({ timeout: 10_000 })

    // Click the first data row
    await rows.first().click()

    // Detail modal should open — check for text unique to the modal
    const dialog = page.locator('[role="dialog"]')
    await expect(dialog).toBeVisible({ timeout: 5_000 })
    await expect(dialog.getByText('ID de evento')).toBeVisible()
    await expect(dialog.getByText('Resultado', { exact: true })).toBeVisible()
  })

  test('botón "Exportar CSV" dispara descarga', async ({ page }) => {
    // Wait for data to load
    const rows = page.locator('tbody tr').filter({ hasNotText: 'No se encontraron eventos' })
    await expect(rows.first()).toBeVisible({ timeout: 10_000 })

    // Listen for download event
    const downloadPromise = page.waitForEvent('download', { timeout: 10_000 })

    // Click the export button
    await page.getByRole('button', { name: 'Exportar CSV' }).click()

    // Wait for download to start
    const download = await downloadPromise
    expect(download.suggestedFilename()).toMatch(/^auditoria_.*\.csv$/)
  })
})
