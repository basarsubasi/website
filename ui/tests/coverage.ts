import { test as base, type Page } from "@playwright/test";
import { coverageReport } from "./coverage-report";

const enabled = process.env.UI_COVERAGE === "1";
const blankPage = "about:blank";
const assetPath = "/assets/";
const buildAssets = new URL("../.vite/coverage-dist/assets/", import.meta.url);
const scriptExtension = ".js";

async function collect(page: Page) {
  const assets = new URL(assetPath, page.url()).href;
  const entries = await page.coverage.stopJSCoverage();
  const bundles = entries.filter(entry => entry.url.startsWith(assets) && entry.url.endsWith(scriptExtension));
  for (const bundle of bundles) {
    // Read the source map for the exact Vite bundle that Chromium executed.
    bundle.url = new URL(bundle.url.slice(assets.length), buildAssets).href;
  }
  if (bundles.length) await coverageReport.add(bundles);
}

async function beforeNavigation(page: Page) {
  // Chromium can discard the old document's counters when a full navigation starts.
  if (enabled && page.url() !== blankPage) {
    await collect(page);
    await page.coverage.startJSCoverage();
  }
}

export async function goto(page: Page, ...args: Parameters<Page["goto"]>) {
  await beforeNavigation(page);
  return page.goto(...args);
}

export async function reload(page: Page) {
  await beforeNavigation(page);
  return page.reload();
}

async function trackPage(page: Page, use: (page: Page) => Promise<void>) {
  if (!enabled) return use(page);
  await page.coverage.startJSCoverage();
  try {
    await use(page);
  } finally {
    if (page.url() !== blankPage) await collect(page);
  }
}

export const test = base.extend<{ publicPage: Page }>({
  page: async ({ page }, use) => trackPage(page, use),
  publicPage: async ({ browser, baseURL }, use) => {
    const context = await browser.newContext({ baseURL });
    try {
      await trackPage(await context.newPage(), use);
    } finally {
      await context.close();
    }
  },
});
