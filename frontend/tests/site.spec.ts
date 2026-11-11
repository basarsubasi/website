import { expect, test, type Page } from "@playwright/test";

async function login(page: Page) {
  await page.goto("/admin");
  await page.getByLabel("Password").fill("ui-test-password-only");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();
}

// Valid 1x1 PNG; the real Go upload decoder validates these bytes.
const png = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGP4z8AAAAMBAQDJ/pLvAAAAAElFTkSuQmCC",
  "base64",
);

test("login errors, session reload, and logout", async ({ page }) => {
  await page.goto("/admin");
  await page.getByLabel("Password").fill("incorrect");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page.getByRole("alert")).toHaveText("Invalid password");
  await login(page);
  await page.reload();
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();
  await page.getByRole("button", { name: "Sign out" }).click();
  await page.reload();
  await expect(page.getByLabel("Password")).toBeVisible();
});

test("upload, view, edit, filter, and delete a photo", async ({ page }) => {
  await login(page);
  await page.getByRole("button", { name: "Add photo", exact: true }).click();
  await page.getByLabel("Photo file").setInputFiles({ name: "canal.png", mimeType: "image/png", buffer: png });
  await page.getByLabel("Name / alt text").fill("Amsterdam canal");
  await page.getByLabel("City", { exact: true }).fill("Amsterdam");
  await page.getByLabel("Country", { exact: true }).fill("Netherlands");
  await page.getByLabel("Year", { exact: true }).fill("2026");
  await page.getByRole("combobox", { name: "Color", exact: true }).selectOption("bw");
  await page.getByRole("button", { name: "Save photo" }).click();
  await expect(page.getByText("Photo saved.", { exact: true })).toBeVisible();

  await page.goto("/bw-photos");
  const image = page.getByRole("img", { name: "Amsterdam canal" });
  await expect(image).toBeVisible();
  await expect.poll(() => image.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(1);
  await image.click();
  await expect(page).toHaveURL(/\/image\//);
  await page.reload();
  await expect(page.locator(".image-metadata")).toBeVisible();
  await expect(page.locator(".image-metadata")).toContainText("Netherlands");

  await page.goto("/admin");
  const row = page.getByRole("listitem").filter({ hasText: "Amsterdam canal" });
  await row.getByRole("button", { name: "Edit", exact: true }).click();
  await page.getByLabel("Name / alt text").fill("Canal in color");
  await page.getByRole("combobox", { name: "Color", exact: true }).selectOption("color");
  await page.getByRole("button", { name: "Save photo" }).click();
  await expect(page.getByText("Photo saved.", { exact: true })).toBeVisible();
  await page.goto("/bw-photos");
  await expect(page.getByText("No photos yet.")).toBeVisible();
  await page.goto("/color-photos");
  await expect(page.getByRole("img", { name: "Canal in color" })).toBeVisible();

  await page.goto("/admin");
  const edited = page.getByRole("listitem").filter({ hasText: "Canal in color" });
  await edited.getByRole("button", { name: "Delete", exact: true }).click();
  await edited.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(edited).toBeVisible();
  await edited.getByRole("button", { name: "Delete", exact: true }).click();
  await edited.getByRole("button", { name: "Delete permanently" }).click();
  await expect(edited).toHaveCount(0);
  await page.goto("/color-photos");
  await expect(page.getByText("No photos yet.")).toBeVisible();
});

test("draft privacy, publishing, editing, deep links, and deletion", async ({ page, browser }) => {
  await login(page);
  await page.getByRole("button", { name: "Blog posts", exact: true }).click();
  await page.getByRole("button", { name: "New post", exact: true }).click();
  await page.getByLabel("Title", { exact: true }).fill("Amsterdam journal");
  await page.getByLabel("Excerpt").fill("A day beside the canals.");
  await expect(page.getByLabel("Slug", { exact: true })).toHaveValue("amsterdam-journal");
  await page.getByLabel("Body", { exact: true }).fill("First paragraph.\n\nSecond paragraph.");
  await page.getByRole("button", { name: "Save post" }).click();
  await expect(page.getByText("Post saved.", { exact: true })).toBeVisible();
  const visitor = await browser.newContext();
  const publicPage = await visitor.newPage();
  try {
    await publicPage.goto("http://127.0.0.1:18080/blog/amsterdam-journal");
    await expect(publicPage.getByRole("alert")).toHaveText("Not found");
    await publicPage.goto("http://127.0.0.1:18080/blog");
    await expect(publicPage.getByRole("heading", { name: "Amsterdam journal" })).toHaveCount(0);

    const row = page.getByRole("listitem").filter({ hasText: "Amsterdam journal" });
    await row.getByRole("button", { name: "Edit", exact: true }).click();
    await page.getByLabel("Status").selectOption("1");
    await page.getByRole("button", { name: "Save post" }).click();
    await expect(page.getByText("Post saved.", { exact: true })).toBeVisible();
    await publicPage.reload();
    await expect(publicPage.getByText("A day beside the canals.")).toBeVisible();
    await publicPage.getByRole("link", { name: "Read post →" }).click();
    await publicPage.reload();
    await expect(publicPage.locator(".post-body")).toHaveText("First paragraph.\n\nSecond paragraph.");

    await row.getByRole("button", { name: "Edit", exact: true }).click();
    await page.getByLabel("Body", { exact: true }).fill("Updated journal.");
    await page.getByRole("button", { name: "Save post" }).click();
    await expect(page.getByText("Post saved.", { exact: true })).toBeVisible();
    await publicPage.reload();
    await expect(publicPage.getByText("Updated journal.", { exact: true })).toBeVisible();
    await row.getByRole("button", { name: "Delete", exact: true }).click();
    await row.getByRole("button", { name: "Delete permanently" }).click();
    await expect(row).toHaveCount(0);
    await publicPage.reload();
    await expect(publicPage.getByRole("alert")).toHaveText("Not found");
  } finally { await visitor.close(); }
});

test("invalid image uploads show a useful error", async ({ page }) => {
  await login(page);
  await page.getByRole("button", { name: "Add photo", exact: true }).click();
  await page.getByLabel("Photo file").setInputFiles({ name: "fake.png", mimeType: "image/png", buffer: Buffer.from("not an image") });
  await page.getByRole("button", { name: "Save photo" }).click();
  await expect(page.getByRole("alert")).toContainText("image");
  await expect(page.getByRole("heading", { name: "Add photo" })).toBeVisible();
});
