import { test, expect } from '@playwright/test'

// Helper: target the main content area to avoid TOC/sidebar duplicates
const article = (page: any) => page.locator('article')
const searchInput = (page: any) => page.getByPlaceholder('Search...').first()
const searchResults = (page: any) =>
  page.getByLabel('Search results').filter({ has: page.locator('a[href]') }).first()
const isGlossaryIndexRoute = (route: string) => /\/reference\/glossary\/?$/.test(route)

test.describe('Homepage', () => {
  test('loads with hero and three section cards', async ({ page }) => {
    await test.step('navigate to homepage', async () => {
      await page.goto('/')
    })

    await test.step('verify hero content', async () => {
      await expect(page.getByText('A methodology for')).toBeVisible()
      await expect(page.getByText('AI-assisted software')).toBeVisible()
    })

    await test.step('verify single primary CTA', async () => {
      await expect(page.getByRole('link', { name: 'Get Started' })).toBeVisible()
    })

    await test.step('verify three section cards', async () => {
      await expect(page.getByRole('heading', { name: /43 Artifacts/ })).toBeVisible()
      await expect(page.getByRole('heading', { name: /18 Cross-Cutting Concerns/ })).toBeVisible()
      await expect(page.getByRole('heading', { name: 'Why HELIX' })).toBeVisible()
    })

    await test.step('capture full-page screenshot', async () => {
      // Asciinema embed loads async, allow some pixel jitter
      await expect(page).toHaveScreenshot('homepage.png', {
        fullPage: true,
        maxDiffPixelRatio: 0.03,
      })
    })
  })
})

test.describe('Why HELIX', () => {
  test('section landing has the four subpages', async ({ page }) => {
    await page.goto('/why/')
    await expect(article(page).getByRole('link', { name: /The Problem/ })).toBeVisible()
    await expect(article(page).getByRole('link', { name: /The Thesis/ })).toBeVisible()
    await expect(article(page).getByRole('link', { name: /Principles/ })).toBeVisible()
    await expect(article(page).getByRole('link', { name: /Who it's for/ })).toBeVisible()
  })

  test('principles page covers all eight', async ({ page }) => {
    await page.goto('/why/principles/')
    await expect(page.getByRole('heading', { name: /1\.\s+Planning and execution/ })).toBeVisible()
    await expect(page.getByRole('heading', { name: /3\.\s+Authority order/ })).toBeVisible()
    await expect(page.getByRole('heading', { name: /6\.\s+Autonomy is supervised/ })).toBeVisible()
    await expect(page.getByRole('heading', { name: /8\.\s+Least power wins/ })).toBeVisible()
  })

  test('thesis page describes the methodology layer', async ({ page }) => {
    await page.goto('/why/the-thesis/')
    await expect(article(page).getByText(/methodology layer/i)).toBeVisible()
    await expect(article(page).getByText(/seven phases/i).first()).toBeVisible()
  })
})

test.describe('Artifacts', () => {
  test('index lists artifacts grouped by phase', async ({ page }) => {
    await page.goto('/artifacts/')
    // Phase headings should be visible (h2)
    await expect(page.getByRole('heading', { name: /Phase 0.*Discover/ })).toBeVisible()
    await expect(page.getByRole('heading', { name: /Phase 1.*Frame/ })).toBeVisible()
    await expect(page.getByRole('heading', { name: /Phase 6.*Iterate/ })).toBeVisible()
  })

  test('artifact pages render generation prompt and template', async ({ page }) => {
    await page.goto('/artifacts/prd/')
    await expect(page.getByRole('heading', { name: /What it is/ })).toBeVisible()
    await expect(page.getByRole('heading', { name: /Relationships/ })).toBeVisible()
    // Prompt and template are wrapped in <details>
    await expect(article(page).getByText(/generation prompt/i).first()).toBeVisible()
    await expect(article(page).getByText(/Show the template structure/i)).toBeVisible()
  })

  test('alias from old /reference/glossary/artifacts/ resolves', async ({ page }) => {
    const response = await page.request.get('/reference/glossary/artifacts/prd/')
    // Hugo aliases are HTML redirects; either 200 (rendered) or 3xx (redirect) is acceptable
    expect([200, 301, 302, 308]).toContain(response.status())
  })

  test('artifact type leaf pages keep parent activity navigation', async ({ page }) => {
    await page.setViewportSize({ width: 1800, height: 1000 })
    const sidebar = page.locator('aside').first()
    const artifactTypeLink = (path: string) => sidebar.locator(`a[href$="${path}"]`)

    await page.goto('/artifact-types/frame/prd/')
    await expect(artifactTypeLink('/artifact-types/frame/')).toBeVisible()
    await expect(artifactTypeLink('/artifact-types/frame/prd/')).toBeVisible()
    await expect(artifactTypeLink('/artifact-types/frame/feature-specification/')).toBeVisible()
    await expect(artifactTypeLink('/artifact-types/frame/pr-faq/')).toBeVisible()
    await expect(artifactTypeLink('/artifact-types/frame/prd/')).toHaveClass(
      /hextra-sidebar-active-item/,
    )
    await expect(
      artifactTypeLink('/artifact-types/frame/prd/')
        .locator('xpath=ancestor::li[1]')
        .locator('a[href^="#"]')
        .first(),
    ).toBeHidden()

    await page.goto('/artifact-types/frame/pr-faq/')
    await expect(artifactTypeLink('/artifact-types/frame/')).toBeVisible()
    await expect(artifactTypeLink('/artifact-types/frame/prd/')).toBeVisible()
    await expect(artifactTypeLink('/artifact-types/frame/feature-specification/')).toBeVisible()
    await expect(artifactTypeLink('/artifact-types/frame/pr-faq/')).toBeVisible()
    await expect(artifactTypeLink('/artifact-types/frame/pr-faq/')).toHaveClass(
      /hextra-sidebar-active-item/,
    )
    await expect(
      artifactTypeLink('/artifact-types/frame/pr-faq/')
        .locator('xpath=ancestor::li[1]')
        .locator('a[href^="#"]')
        .first(),
    ).toBeHidden()
  })
})

test.describe('Concerns', () => {
  test('index lists concerns by category', async ({ page }) => {
    await page.goto('/concerns/')
    await expect(page.getByRole('heading', { name: 'Tech Stack' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Quality Attributes' })).toBeVisible()
    // Sample concern cards
    await expect(article(page).getByRole('link', { name: /TypeScript/ })).toBeVisible()
    await expect(article(page).getByRole('link', { name: /a11y|Accessibility/i })).toBeVisible()
  })

  test('concern detail page shows description and practices', async ({ page }) => {
    await page.goto('/concerns/typescript-bun/')
    await expect(page.getByRole('heading', { name: /Description/ })).toBeVisible()
    await expect(page.getByRole('heading', { name: /Practices by phase/ })).toBeVisible()
    await expect(article(page).getByText(/Bun 1\.x/)).toBeVisible()
  })
})

test.describe('Use HELIX', () => {
  test('section landing introduces the how-to layer', async ({ page }) => {
    await page.goto('/use/')
    await expect(article(page).getByRole('link', { name: /Getting Started/ })).toBeVisible()
    await expect(article(page).getByRole('link', { name: /The Workflow/ })).toBeVisible()
  })

  test('Getting Started page has installation instructions', async ({ page }) => {
    await page.goto('/use/getting-started/')
    await expect(article(page).getByText('ddx install helix')).toBeVisible()
    await expect(article(page).getByText('DDx and HELIX')).toBeVisible()
  })

  test('Workflow page explains phases', async ({ page }) => {
    await page.goto('/use/workflow/')
    await expect(page.getByRole('heading', { name: 'Workflow' }).first()).toBeVisible()
  })
})

test.describe('Reference', () => {
  test('section landing lists subsections', async ({ page }) => {
    await page.goto('/reference/')
    await expect(article(page).getByRole('link', { name: /Glossary/ })).toBeVisible()
    await expect(article(page).getByRole('link', { name: /CLI Reference/ })).toBeVisible()
    await expect(article(page).getByRole('link', { name: /Skills/ })).toBeVisible()
    await expect(article(page).getByRole('link', { name: /Demos/ })).toBeVisible()
  })

  test('CLI Reference page documents commands', async ({ page }) => {
    await page.goto('/reference/cli/')
    await expect(page.getByRole('heading', { name: 'CLI Reference' }).first()).toBeVisible()
  })

  test('Skills page lists agent skills', async ({ page }) => {
    await page.goto('/reference/skills/')
    await expect(page.getByRole('heading', { name: 'Skills' }).first()).toBeVisible()
  })

  test('Demos page shows reels', async ({ page }) => {
    await page.goto('/reference/demos/')
    await expect(article(page).getByText('Quickstart: Intake to Queue Drain')).toBeVisible()
    await expect(article(page).getByText('Concerns: Preventing Technology Drift')).toBeVisible()
  })

  test.describe('Glossary', () => {
    test('index lists categories (without Artifacts/Concerns — those are top-level now)', async ({
      page,
    }) => {
      await page.goto('/reference/glossary/')
      await expect(article(page).getByRole('link', { name: /Phases/ })).toBeVisible()
      await expect(article(page).getByRole('link', { name: /Actions/ })).toBeVisible()
      await expect(article(page).getByRole('link', { name: /Concepts/ })).toBeVisible()
      await expect(article(page).getByRole('link', { name: /Tracker/ })).toBeVisible()
    })

    test('phases page covers all seven phases with artifact links', async ({ page }) => {
      await page.goto('/reference/glossary/phases/')
      await expect(page.getByRole('heading', { name: /Phase 0.*Discover/ })).toBeVisible()
      await expect(page.getByRole('heading', { name: /Phase 1.*Frame/ })).toBeVisible()
      await expect(page.getByRole('heading', { name: /Phase 4.*Build/ })).toBeVisible()
      await expect(page.getByRole('heading', { name: /Phase 6.*Iterate/ })).toBeVisible()
      // Artifact links inside phase tables
      await expect(article(page).getByRole('link', { name: 'PRD' })).toBeVisible()
      await expect(article(page).getByRole('link', { name: /ADR/ })).toBeVisible()
    })

    test('actions page documents commands', async ({ page }) => {
      await page.goto('/reference/glossary/actions/')
      await expect(article(page).getByRole('heading', { name: 'build' })).toBeVisible()
      await expect(article(page).getByRole('heading', { name: 'check' })).toBeVisible()
      await expect(article(page).getByRole('heading', { name: 'run' })).toBeVisible()
    })

    test('concepts page covers core ideas', async ({ page }) => {
      await page.goto('/reference/glossary/concepts/')
      await expect(page.getByRole('heading', { name: 'Authority Order' })).toBeVisible()
      await expect(page.getByRole('heading', { name: 'Context Digest' })).toBeVisible()
    })

    test('tracker page documents beads', async ({ page }) => {
      await page.goto('/reference/glossary/tracker/')
      await expect(article(page).getByText('ddx bead').first()).toBeVisible()
      await expect(article(page).getByRole('heading', { name: 'Phase Labels' })).toBeVisible()
    })
  })
})

test.describe('Backward-compat aliases', () => {
  // Hugo aliases are typically HTML-meta redirects (200 with refresh) or 3xx
  const acceptable = [200, 301, 302, 308]

  test('legacy /docs/* URLs still resolve', async ({ page }) => {
    for (const url of [
      '/docs/',
      '/docs/background/',
      '/docs/getting-started/',
      '/docs/workflow/',
      '/docs/cli/',
      '/docs/skills/',
      '/docs/glossary/',
      '/docs/glossary/phases/',
      '/docs/glossary/artifacts/',
      '/docs/glossary/concerns/',
      '/docs/demos/',
    ]) {
      const response = await page.request.get(url)
      expect(acceptable, `${url} should be reachable`).toContain(response.status())
    }
  })
})

test.describe('Navigation Workflows', () => {
  test('homepage → Get Started → use/getting-started', async ({ page }) => {
    await page.goto('/')
    await page.getByRole('link', { name: 'Get Started' }).click()
    await expect(page).toHaveURL(/\/use\/getting-started/)
  })

  test('top nav: Why / Artifacts / Concerns / Use / Reference', async ({ page }) => {
    await page.goto('/')
    const nav = page.getByRole('navigation').first()

    await test.step('Why HELIX', async () => {
      await nav.getByRole('link', { name: 'Why HELIX' }).first().click()
      await expect(page).toHaveURL(/\/why\/?$/)
    })

    await test.step('Artifacts', async () => {
      await page.goto('/')
      await nav.getByRole('link', { name: 'Artifacts' }).first().click()
      await expect(page).toHaveURL(/\/artifacts\/?$/)
    })

    await test.step('Concerns', async () => {
      await page.goto('/')
      await nav.getByRole('link', { name: 'Concerns' }).first().click()
      await expect(page).toHaveURL(/\/concerns\/?$/)
    })

    await test.step('Use HELIX', async () => {
      await page.goto('/')
      await nav.getByRole('link', { name: 'Use HELIX' }).first().click()
      await expect(page).toHaveURL(/\/use\/?$/)
    })

    await test.step('Reference', async () => {
      await page.goto('/')
      await nav.getByRole('link', { name: 'Reference' }).first().click()
      await expect(page).toHaveURL(/\/reference\/?$/)
    })
  })

  test('artifacts → individual artifact drill-down', async ({ page }) => {
    await page.goto('/artifacts/')
    await article(page).getByRole('link', { name: /^PRD/ }).first().click()
    await expect(page).toHaveURL(/\/artifacts\/prd\/?$/)
    await expect(page.getByRole('heading', { name: /What it is/ })).toBeVisible()
  })
})

test.describe('Search Workflows', () => {
  test('search index points glossary queries at /reference/glossary/', async ({ page }) => {
    const response = await page.request.get('/en.search-data.json')
    expect(response.ok()).toBeTruthy()

    const searchIndex = (await response.json()) as Record<string, unknown>
    const glossaryRoutes = Object.keys(searchIndex).filter(route => route.endsWith('/glossary/'))

    // Exactly one glossary index, at the new location
    expect(glossaryRoutes.filter(isGlossaryIndexRoute)).toHaveLength(1)
    expect(glossaryRoutes.filter(route => !isGlossaryIndexRoute(route))).toEqual([])
  })

  test('search opens from menu and surfaces glossary results', async ({ page }) => {
    await page.setViewportSize({ width: 700, height: 900 })
    await page.goto('/use/')

    await page.getByRole('button', { name: 'Menu' }).click()
    await expect(searchInput(page)).toBeVisible()
    await searchInput(page).click()
    await expect(searchInput(page)).toBeFocused()

    await searchInput(page).pressSequentially('glossary')
    await expect(searchResults(page)).toBeVisible()

    const result = searchResults(page).locator('a[href*="/reference/glossary"]').first()
    await expect(result).toBeVisible()
    await result.click()
    await expect(page).toHaveURL(/\/reference\/glossary(\/|#|$)/)
  })
})
