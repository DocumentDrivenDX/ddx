import { test } from '@playwright/test'

// DDx Server UI — Demo Recording (TP-002 TC-009)
//
// Produces a polished video walkthrough of all 6 pages with real data
// interactions. Designed for embedding in the DDx microsite.
//
// Run:
//   bun run demo:record
// Output:
//   demo-output/ contains a .webm video file

test.describe('DDx Server UI Demo', () => {
  test('full walkthrough', async ({ page }) => {
    // ---------------------------------------------------------------
    // 1. Dashboard — overview of the project
    // ---------------------------------------------------------------
    await page.goto('/')
    await page.waitForSelector('h1')
    // Wait for all API data to populate cards
    await page.waitForSelector('text=ok', { timeout: 5000 })
    await page.waitForTimeout(2500)

    // ---------------------------------------------------------------
    // 2. Documents — browse and read a document
    // ---------------------------------------------------------------
    await page.locator('a[href="/documents"]').first().click()
    await page.waitForSelector('h1:has-text("Documents")')
    await page.waitForTimeout(1000)

    // Select the first document to show rendered markdown
    const firstDoc = page.locator('.overflow-auto button').first()
    if (await firstDoc.isVisible({ timeout: 3000 })) {
      await firstDoc.click()
      await page.waitForSelector('.prose', { timeout: 5000 })
      await page.waitForTimeout(2000)

      // Show the edit toggle briefly
      const editBtn = page.locator('button:has-text("Edit")')
      if (await editBtn.isVisible()) {
        await editBtn.click()
        await page.waitForTimeout(1200)
        await page.click('button:has-text("Cancel")')
        await page.waitForTimeout(800)
      }
    }

    // Demonstrate type filtering
    const typeSelect = page.locator('select')
    const options = await typeSelect.locator('option').allTextContents()
    if (options.length > 1) {
      await typeSelect.selectOption({ index: 1 })
      await page.waitForTimeout(1000)
      await typeSelect.selectOption({ index: 0 }) // back to "All types"
      await page.waitForTimeout(500)
    }

    // Demonstrate search
    const docSearch = page.locator('input[placeholder*="Search"]')
    if (await docSearch.isVisible()) {
      await docSearch.fill('persona')
      await page.waitForTimeout(1000)
      await docSearch.fill('')
      await page.waitForTimeout(500)
    }

    // ---------------------------------------------------------------
    // 3. Beads — kanban board, search, detail, create
    // ---------------------------------------------------------------
    await page.locator('a[href="/beads"]').first().click()
    await page.waitForSelector('text=OPEN')
    await page.waitForTimeout(1500)

    // Search for beads
    const beadSearch = page.locator('input[placeholder*="Search beads"]')
    if (await beadSearch.isVisible()) {
      await beadSearch.fill('helix')
      await page.waitForTimeout(1200)
      await beadSearch.fill('')
      await page.waitForTimeout(600)
    }

    // Click a bead card to show detail panel
    const beadCard = page.locator('[draggable="true"]').first()
    if (await beadCard.isVisible()) {
      await beadCard.click()
      await page.waitForTimeout(2000)
      // Close detail
      const closeBtn = page.locator('button:has-text("×")')
      if (await closeBtn.isVisible()) {
        await closeBtn.click()
        await page.waitForTimeout(500)
      }
    }

    // Show the create bead modal (don't submit — just demonstrate the form)
    const newBeadBtn = page.locator('button:has-text("New Bead")')
    if (await newBeadBtn.isVisible()) {
      await newBeadBtn.click()
      await page.waitForSelector('form')
      await page.waitForTimeout(800)

      const titleInput = page.locator('form input[type="text"]').first()
      await titleInput.fill('Demo: example work item')
      await page.waitForTimeout(600)

      const descriptionArea = page.locator('form textarea').first()
      await descriptionArea.fill('Created during the DDx server UI demo walkthrough.')
      await page.waitForTimeout(800)

      // Close the modal without submitting
      await page.click('button:has-text("Cancel")')
      await page.waitForTimeout(500)
    }

    // ---------------------------------------------------------------
    // 4. Graph — document dependency visualization
    // ---------------------------------------------------------------
    await page.locator('a[href="/graph"]').first().click()
    await page.waitForTimeout(2500)

    // ---------------------------------------------------------------
    // 5. Agent — session history
    // ---------------------------------------------------------------
    await page.locator('a[href="/agent"]').first().click()
    await page.waitForTimeout(2000)

    // ---------------------------------------------------------------
    // 6. Personas — browse and view a persona
    // ---------------------------------------------------------------
    await page.locator('a[href="/personas"]').first().click()
    await page.waitForTimeout(2000)

    const firstPersona = page.locator('.w-80 button').first()
    if (await firstPersona.isVisible({ timeout: 2000 }).catch(() => false)) {
      await firstPersona.click()
      await page.waitForTimeout(2000)
    }

    // ---------------------------------------------------------------
    // 7. Back to Dashboard — closing shot
    // ---------------------------------------------------------------
    await page.locator('a[href="/"]').first().click()
    await page.waitForSelector('h1')
    await page.waitForTimeout(2000)
  })
})
