import { expect, test, type Page } from "@playwright/test";

async function login(page: Page) {
  await page.goto("/admin");
  await page.getByLabel("Şifre").fill("ui-test-password-only");
  await page.getByRole("button", { name: "Giriş yap", exact: true }).click();
  await expect(page.getByRole("button", { name: "Çıkış yap" })).toBeVisible();
}

// Valid 1x1 PNG; the real Go upload decoder validates these bytes.
const png = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGP4z8AAAAMBAQDJ/pLvAAAAAElFTkSuQmCC",
  "base64",
);

test("gallery and blog routes render without a site header", async ({ page }) => {
  for (const path of ["/photos", "/bw-photos", "/color-photos", "/blog", "/blog/missing-post"]) {
    await page.goto(path);
    await expect(page.locator("main")).toBeVisible();
    await expect(page.getByRole("banner")).toHaveCount(0);
  }
});

test("login errors, session reload, and logout", async ({ page }) => {
  await page.goto("/admin");
  await page.getByLabel("Şifre").fill("incorrect");
  await page.getByRole("button", { name: "Giriş yap", exact: true }).click();
  await expect(page.getByRole("alert")).toHaveText("Şifre yanlış.");
  await login(page);
  await page.reload();
  await expect(page.getByRole("button", { name: "Çıkış yap" })).toBeVisible();
  await page.getByRole("button", { name: "Çıkış yap" }).click();
  await page.reload();
  await expect(page.getByLabel("Şifre")).toBeVisible();
});

test("upload, view, edit, filter, and delete a photo", async ({ page }) => {
  await login(page);
  await page.getByRole("button", { name: "Fotoğraf ekle", exact: true }).click();
  await page.getByLabel("Fotoğraf dosyası").setInputFiles({ name: "canal.png", mimeType: "image/png", buffer: png });
  await page.getByLabel("Ad / alternatif metin").fill("Amsterdam canal");
  await page.getByLabel("Şehir", { exact: true }).fill("Amsterdam");
  await page.getByLabel("Ülke", { exact: true }).fill("Netherlands");
  await page.getByLabel("Yıl", { exact: true }).fill("2026");
  await page.getByRole("combobox", { name: "Renk", exact: true }).selectOption("bw");
  await page.getByRole("button", { name: "Fotoğrafı kaydet" }).click();
  await expect(page.getByText("Fotoğraf kaydedildi.", { exact: true })).toBeVisible();

  await page.goto("/bw-photos");
  const image = page.getByRole("img", { name: "Amsterdam canal" });
  await expect(image).toBeVisible();
  await expect.poll(() => image.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(1);
  await image.click();
  await expect(page).toHaveURL(/\/photo\//);
  await page.reload();
  await expect(page.locator(".single-image-page img")).toBeVisible();
  await expect(page.locator(".single-image-page")).toHaveText("");

  await page.goto("/admin");
  const row = page.getByRole("listitem").filter({ hasText: "Amsterdam canal" });
  await row.getByRole("button", { name: "Düzenle", exact: true }).click();
  await page.getByLabel("Ad / alternatif metin").fill("Canal in color");
  await page.getByRole("combobox", { name: "Renk", exact: true }).selectOption("color");
  await page.getByRole("button", { name: "Fotoğrafı kaydet" }).click();
  await expect(page.getByText("Fotoğraf kaydedildi.", { exact: true })).toBeVisible();
  await page.goto("/bw-photos");
  await expect(page.getByText("Henüz fotoğraf yok.")).toBeVisible();
  await page.goto("/color-photos");
  await expect(page.getByRole("img", { name: "Canal in color" })).toBeVisible();

  await page.goto("/admin");
  const edited = page.getByRole("listitem").filter({ hasText: "Canal in color" });
  await edited.getByRole("button", { name: "Sil", exact: true }).click();
  await edited.getByRole("button", { name: "Vazgeç", exact: true }).click();
  await expect(edited).toBeVisible();
  await edited.getByRole("button", { name: "Sil", exact: true }).click();
  await edited.getByRole("button", { name: "Kalıcı olarak sil" }).click();
  await expect(edited).toHaveCount(0);
  await page.goto("/color-photos");
  await expect(page.getByText("Henüz fotoğraf yok.")).toBeVisible();
});

test("draft privacy, publishing, editing, deep links, and deletion", async ({ page, browser }) => {
  await login(page);
  await page.getByRole("button", { name: "Blog yazıları", exact: true }).click();
  await page.getByRole("button", { name: "Yeni yazı", exact: true }).click();
  await page.getByLabel("Başlık", { exact: true }).fill("İstanbul kış günlüğü");
  await page.getByRole("button", { name: "Yazı ayarları" }).click();
  await page.getByLabel("Özet").fill("A day beside the canals.");
  await expect(page.getByLabel("Kısa bağlantı", { exact: true })).toHaveValue("istanbul-kis-gunlugu");
  await page.getByLabel("İçerik", { exact: true }).fill("First paragraph.\n\nSecond paragraph.");
  await page.getByRole("button", { name: "Yazıyı kaydet" }).click();
  await expect(page.getByText("Yazı kaydedildi.", { exact: true })).toBeVisible();
  const visitor = await browser.newContext();
  const publicPage = await visitor.newPage();
  try {
    await publicPage.goto("http://127.0.0.1:18080/blog/istanbul-kis-gunlugu");
    await expect(publicPage.getByRole("alert")).toHaveText("İçerik bulunamadı.");
    await publicPage.goto("http://127.0.0.1:18080/blog");
    await expect(publicPage.getByRole("heading", { name: "İstanbul kış günlüğü" })).toHaveCount(0);

    const row = page.getByRole("listitem").filter({ hasText: "İstanbul kış günlüğü" });
    await row.getByRole("button", { name: "Düzenle", exact: true }).click();
    await page.getByRole("button", { name: "Yazı ayarları" }).click();
    await page.getByLabel("Durum").selectOption("1");
    await page.getByRole("button", { name: "Yazıyı kaydet" }).click();
    await expect(page.getByText("Yazı kaydedildi.", { exact: true })).toBeVisible();
    await publicPage.reload();
    await expect(publicPage.getByText("A day beside the canals.")).toBeVisible();
    await publicPage.getByRole("link", { name: "İstanbul kış günlüğü", exact: true }).click();
    await publicPage.reload();
    await expect(publicPage.locator(".post-body p")).toHaveText(["First paragraph.", "Second paragraph."]);

    await row.getByRole("button", { name: "Düzenle", exact: true }).click();
    await page.getByLabel("İçerik", { exact: true }).fill("Updated journal.");
    await page.getByRole("button", { name: "Yazıyı kaydet" }).click();
    await expect(page.getByText("Yazı kaydedildi.", { exact: true })).toBeVisible();
    await publicPage.reload();
    await expect(publicPage.getByText("Updated journal.", { exact: true })).toBeVisible();
    await row.getByRole("button", { name: "Sil", exact: true }).click();
    await row.getByRole("button", { name: "Kalıcı olarak sil" }).click();
    await expect(row).toHaveCount(0);
    await publicPage.reload();
    await expect(publicPage.getByRole("alert")).toHaveText("İçerik bulunamadı.");
  } finally { await visitor.close(); }
});

test("invalid image uploads show a useful error", async ({ page }) => {
  await login(page);
  await page.getByRole("button", { name: "Fotoğraf ekle", exact: true }).click();
  await page.getByLabel("Fotoğraf dosyası").setInputFiles({ name: "fake.png", mimeType: "image/png", buffer: Buffer.from("not an image") });
  await page.getByRole("button", { name: "Fotoğrafı kaydet" }).click();
  await expect(page.getByRole("alert")).toContainText("fotoğraf");
  await expect(page.getByRole("heading", { name: "Fotoğraf ekle" })).toBeVisible();
});

async function postLayout(page: Page) {
  return page.locator(".post-article").evaluate((article) => {
    const title = article.querySelector(".post-title")!;
    const body = article.querySelector(".post-body")!;
    const paragraph = body.querySelector("p")!;
    const style = (el: Element) => {
      const css = getComputedStyle(el);
      return { font: css.fontFamily, size: css.fontSize, weight: css.fontWeight, lineHeight: css.lineHeight, width: el.getBoundingClientRect().width };
    };
    return {
      title: style(title), body: style(body), paragraph: style(paragraph),
      paragraphGap: getComputedStyle(paragraph).marginBottom,
      headingGap: body.getBoundingClientRect().top - article.querySelector(".post-heading")!.getBoundingClientRect().bottom,
    };
  });
}

test("inline editor matches the published layout and preserves paragraphs and line breaks", async ({ page }) => {
  await login(page);
  await page.getByRole("button", { name: "Blog yazıları", exact: true }).click();
  await page.getByRole("button", { name: "Yeni yazı", exact: true }).click();
  await page.getByLabel("Başlık", { exact: true }).fill("İstanbul sokaklarında bir sonbahar sabahı");
  const body = page.getByRole("textbox", { name: "İçerik", exact: true });
  await body.click();
  await page.keyboard.insertText("İlk paragraf. Sokaklarda yürürken ışığın ve gölgelerin değişimini izledim.");
  await page.keyboard.press("Enter");
  await page.keyboard.insertText("İkinci paragraf.");
  await page.keyboard.press("Shift+Enter");
  await page.keyboard.insertText("Aynı paragrafta yeni satır.");
  await expect(body.locator("p")).toHaveCount(2);
  await expect(body.locator("br:not(.ProseMirror-trailingBreak)")).toHaveCount(1);
  const layouts = [];
  for (const width of [1280, 375]) {
    await page.setViewportSize({ width, height: 900 });
    layouts.push(await postLayout(page));
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  }
  await page.getByRole("button", { name: "Yazı ayarları" }).click();
  await page.getByLabel("Durum").selectOption("1");
  await page.getByRole("button", { name: "Yazıyı kaydet" }).click();
  await expect(page.getByText("Yazı kaydedildi.", { exact: true })).toBeVisible();
  await page.goto("/blog/istanbul-sokaklarinda-bir-sonbahar-sabahi");
  await expect(page.locator(".post-body p")).toHaveText([
    "İlk paragraf. Sokaklarda yürürken ışığın ve gölgelerin değişimini izledim.",
    "İkinci paragraf.\nAynı paragrafta yeni satır.",
  ]);
  for (const [index, width] of [1280, 375].entries()) {
    await page.setViewportSize({ width, height: 900 });
    expect(await postLayout(page)).toEqual(layouts[index]);
  }
  await page.goto("/admin");
  await page.getByRole("button", { name: "Blog yazıları", exact: true }).click();
  const row = page.getByRole("listitem").filter({ hasText: "İstanbul sokaklarında bir sonbahar sabahı" });
  await row.getByRole("button", { name: "Düzenle", exact: true }).click();
  await expect(body.locator("p")).toHaveCount(2);
  await expect(body.locator("br:not(.ProseMirror-trailingBreak)")).toHaveCount(1);
  await page.getByRole("button", { name: "Vazgeç", exact: true }).click();
  await row.getByRole("button", { name: "Sil", exact: true }).click();
  await row.getByRole("button", { name: "Kalıcı olarak sil" }).click();
});

test("inline editor validates empty content and supports safe paste and undo", async ({ page }) => {
  await login(page);
  await page.getByRole("button", { name: "Blog yazıları", exact: true }).click();
  await page.getByRole("button", { name: "Yeni yazı", exact: true }).click();
  await page.getByRole("button", { name: "Yazıyı kaydet" }).click();
  await expect(page.getByRole("alert")).toHaveText("Bir başlık yazın.");
  await page.getByLabel("Başlık", { exact: true }).fill("Deneme");
  await page.getByRole("button", { name: "Yazıyı kaydet" }).click();
  await expect(page.getByRole("alert")).toHaveText("Yazınızın içeriğini ekleyin.");
  const body = page.getByRole("textbox", { name: "İçerik", exact: true });
  await body.evaluate((el) => {
    const data = new DataTransfer();
    data.setData("text/plain", "Birinci paragraf.\n\nİkinci paragraf.");
    data.setData("text/html", '<p><b>Birinci paragraf.</b></p><img src="x" onerror="window.pasteExecuted=true"><p>İkinci paragraf.</p>');
    el.dispatchEvent(new ClipboardEvent("paste", { clipboardData: data, bubbles: true, cancelable: true }));
  });
  await expect(body.locator("p")).toHaveText(["Birinci paragraf.", "İkinci paragraf."]);
  await expect(body.locator("img, script, b, strong")).toHaveCount(0);
  await body.press("ControlOrMeta+z");
  await expect(body).toHaveText("");
  await body.press("ControlOrMeta+Shift+z");
  await expect(body.locator("p")).toHaveText(["Birinci paragraf.", "İkinci paragraf."]);
  await page.getByRole("button", { name: "Vazgeç", exact: true }).click();
});

test("blog images retain width and captions after saving", async ({ page }) => {
  await login(page);
  await page.getByRole("button", { name: "Blog yazıları", exact: true }).click();
  await page.getByRole("button", { name: "Yeni yazı", exact: true }).click();
  await page.getByLabel("Başlık", { exact: true }).fill("Image journal");
  await page.getByLabel("İçerik", { exact: true }).fill("Before the image.");
  await page.locator('.post-editor input[type="file"]').setInputFiles({ name: "blog.png", mimeType: "image/png", buffer: png });
  const figure = page.locator(".tiptap figure");
  await expect(figure.locator("img")).toHaveAttribute("src", /\/blog\/images\/[^/]+$/);
  await expect(figure.locator("img")).toHaveAttribute("width", "1");
  await expect(figure.locator("img")).toHaveAttribute("height", "1");
  await expect(figure.getByRole("button", { name: "Görseli sil", exact: true })).toBeHidden();
  const captionLayout = () => figure.locator("figcaption").evaluate(el => ({
    width: el.getBoundingClientRect().width, font: getComputedStyle(el).fontSize,
  }));
  const originalCaption = await captionLayout();
  await figure.locator("img").click();
  await expect(figure.getByRole("button", { name: "Görseli sil", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "%25", exact: true }).click();
  expect(await captionLayout()).toEqual(originalCaption);
  await page.getByRole("button", { name: "%50", exact: true }).click();
  await expect(figure.locator(".blog-image-frame")).toHaveAttribute("style", /width: 50%/);
  await figure.locator("figcaption").click();
  await page.keyboard.type("A quiet afternoon.");
  await page.getByRole("button", { name: "Yazı ayarları" }).click();
  await page.getByLabel("Durum").selectOption("1");
  await page.getByRole("button", { name: "Yazıyı kaydet" }).click();
  await expect(page.getByText("Yazı kaydedildi.", { exact: true })).toBeVisible();
  await page.getByRole("listitem").filter({ hasText: "Image journal" }).getByRole("button", { name: "Düzenle", exact: true }).click();
  await expect(figure.locator(".blog-image-frame")).toHaveAttribute("style", /width: 50%/);
  await expect(figure.locator("figcaption")).toHaveText("A quiet afternoon.");
  await page.goto("/blog/image-journal");
  await page.reload();
  await expect(page.locator("figure img")).toHaveAttribute("style", /width: 50%/);
  await expect(page.locator("figcaption")).toHaveText("A quiet afternoon.");
  await expect(page.locator("figcaption")).toHaveCSS("font-style", "italic");
  await expect(page.locator("figure img")).toHaveAttribute("width", "1");
  await expect(page.locator("figure img")).toHaveAttribute("height", "1");
  await expect(page.locator("figure img")).toHaveAttribute("loading", "eager");
  const caption = page.locator("figure figcaption");
  expect((await caption.boundingBox())!.width).toBeGreaterThan((await page.locator("figure img").boundingBox())!.width);
  const src = await page.locator("figure img").getAttribute("src");
  await page.goto("/admin");
  await page.getByRole("button", { name: "Blog yazıları", exact: true }).click();
  await page.getByRole("listitem").filter({ hasText: "Image journal" }).getByRole("button", { name: "Düzenle", exact: true }).click();
  await figure.locator("img").click();
  await page.route("**/api/admin/blog/images/*", route => route.fulfill({ status: 503, json: { error: "Storage unavailable" } }));
  await figure.getByRole("button", { name: "Görseli sil", exact: true }).click();
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(figure.locator("img")).toBeVisible();
  await page.unroute("**/api/admin/blog/images/*");
  await figure.getByRole("button", { name: "Görseli sil", exact: true }).click();
  await expect(figure).toHaveCount(0);
  expect((await page.request.get(src!)).status()).toBe(404);
  await page.getByLabel("İçerik", { exact: true }).press("ControlOrMeta+z");
  await expect(figure).toHaveCount(0);
  await page.getByRole("button", { name: "Yazıyı kaydet" }).click();
  await expect(page.getByText("Yazı kaydedildi.", { exact: true })).toBeVisible();
  await page.goto("/blog/image-journal");
  await expect(page.locator("figure")).toHaveCount(0);
});


test("image loading priorities and dimensions reserve space before downloads", async ({ page }) => {
  await page.route("https://images.example.com/**", route => route.abort());
  const photos = Array.from({ length: 9 }, (_, index) => ({
    uuid: `photo-${index}`, image_url: `https://images.example.com/${index}.png`,
    width: 1200, height: 800, name: `Photo ${index}`, color: "color",
  }));
  await page.route("**/api/images?*", route => route.fulfill({ json: { data: photos, total: 9 } }));
  await page.goto("/photos");
  const thumbnails = page.locator(".grid img");
  await expect(thumbnails).toHaveCount(9);
  for (let index = 0; index < 9; index++) {
    await expect(thumbnails.nth(index)).toHaveAttribute("loading", index < 3 ? "eager" : "lazy");
    await expect(thumbnails.nth(index)).toHaveAttribute("decoding", "async");
  }
  await expect(thumbnails.first()).toHaveAttribute("fetchpriority", "high");
  await page.route("**/api/images/photo-0", route => route.fulfill({ json: { data: photos[0] } }));
  await thumbnails.first().click();
  const main = page.locator(".single-image-page img");
  await expect(main).toHaveAttribute("fetchpriority", "high");
  await expect(main).toHaveAttribute("loading", "eager");
  await expect(main).toHaveAttribute("width", "1200");
  await expect(main).toHaveAttribute("height", "800");
  const body = "blog-content-v1:" + JSON.stringify([
    { type: "blogImage", attrs: { src: photos[0].image_url, width: 50, intrinsicWidth: 1200, intrinsicHeight: 800 } },
    { type: "paragraph", content: [{ type: "text", text: "Below the first image." }] },
    { type: "blogImage", attrs: { src: photos[1].image_url, width: 100, intrinsicWidth: 1200, intrinsicHeight: 800 } },
  ]);
  await page.route("**/api/posts/loading-policy", route => route.fulfill({ json: { data: {
    title: "Loading policy", body, created_at: "2026-01-01T00:00:00Z",
  } } }));
  await page.goto("/blog/loading-policy");
  const images = page.locator(".blog-image img");
  await expect(images.first()).toHaveAttribute("loading", "eager");
  await expect(images.first()).toHaveAttribute("fetchpriority", "high");
  await expect(images.nth(1)).toHaveAttribute("loading", "lazy");
  await expect(images.nth(1)).toHaveAttribute("decoding", "async");
  const bounds = await images.first().boundingBox();
  expect(bounds?.height).toBeGreaterThan(100);
  expect(bounds!.width / bounds!.height).toBeCloseTo(1.5, 1);
});

test("admin arrows save gallery order across pages and reloads", async ({ page }) => {
  await login(page);
  const ids: string[] = [];
  try {
    for (let i = 0; i < 21; i++) {
      const response = await page.request.post("/api/admin/images", { multipart: {
        photo: { name: "order.png", mimeType: "image/png", buffer: png },
        metadata: JSON.stringify({ name: `Order ${i}`, color: "bw", year_taken: 2026 }),
      } });
      expect(response.ok()).toBeTruthy();
      ids.push((await response.json()).data.uuid);
    }
    await page.reload();
    const rows = page.locator(".admin-list > li");
    await expect(rows.first()).toContainText("Order 20");
    await expect(rows.first().getByRole("button", { name: "Yukarı taşı" })).toBeDisabled();
    await rows.first().getByRole("button", { name: "Aşağı taşı" }).click();
    await expect(rows.first()).toContainText("Order 19");
    await page.reload();
    await expect(rows.first()).toContainText("Order 19");
    await rows.last().getByRole("button", { name: "Aşağı taşı" }).click();
    await expect(rows.first()).toContainText("Order 1");
    await expect(rows.first().getByRole("button", { name: "Aşağı taşı" })).toBeDisabled();
    await rows.first().getByRole("button", { name: "Yukarı taşı" }).click();
    await expect(rows).toHaveCount(20);
    await expect(rows.last()).toContainText("Order 1");
    await page.setViewportSize({ width: 375, height: 900 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    for (const path of ["/photos", "/bw-photos"]) {
      await page.goto(path);
      await expect(page.locator(".grid img").first()).toHaveAttribute("alt", "Order 19");
      await expect(page.locator(".grid img").nth(1)).toHaveAttribute("alt", "Order 20");
    }
  } finally {
    for (const id of ids) await page.request.delete("/api/admin/images/" + id);
  }
});

for (const kind of ["photo", "blog"] as const) {
  test(`${kind} editor warns before discarding unsaved changes`, async ({ page }) => {
    await login(page);
    if (kind === "blog") await page.getByRole("button", { name: "Blog yazıları", exact: true }).click();
    const add = page.getByRole("button", { name: kind === "photo" ? "Fotoğraf ekle" : "Yeni yazı", exact: true });
    await add.click();
    const cancel = page.getByRole("button", { name: "Vazgeç", exact: true });
    // Untouched forms close without prompting.
    await cancel.click();
    await expect(add).toBeVisible();
    await add.click();
    const field = page.getByLabel(kind === "photo" ? "Ad / alternatif metin" : "Başlık", { exact: true });
    await field.fill("Unsaved changes");
    let prompt = page.waitForEvent("dialog");
    await page.evaluate(() => { setTimeout(() => location.reload(), 0); });
    let dialog = await prompt;
    expect(dialog.type()).toBe("beforeunload");
    await dialog.dismiss();
    await expect(field).toHaveText(kind === "blog" ? "Unsaved changes" : "");
    if (kind === "photo") await expect(field).toHaveValue("Unsaved changes");

    prompt = page.waitForEvent("dialog");
    const cancelClick = cancel.click();
    dialog = await prompt;
    expect(dialog.type()).toBe("confirm");
    expect(dialog.message()).toContain("Kaydedilmemiş");
    await dialog.dismiss();
    await cancelClick;
    await expect(field).toBeVisible();

    if (kind === "photo") {
      prompt = page.waitForEvent("dialog");
      const switchTab = page.getByRole("button", { name: "Blog yazıları", exact: true }).click();
      await (await prompt).dismiss();
      await switchTab;
      await expect(field).toBeVisible();
      prompt = page.waitForEvent("dialog");
      const logout = page.getByRole("button", { name: "Çıkış yap", exact: true }).click();
      await (await prompt).dismiss();
      await logout;
      await expect(field).toBeVisible();
    }

    prompt = page.waitForEvent("dialog");
    const discard = cancel.click();
    await (await prompt).accept();
    await discard;
    await expect(add).toBeVisible();
    // The listener is removed after leaving the editor.
    expect(await page.evaluate(() => {
      const event = new Event("beforeunload", { cancelable: true });
      return window.dispatchEvent(event);
    })).toBe(true);
  });
}
