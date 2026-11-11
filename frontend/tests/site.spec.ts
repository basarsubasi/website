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
  for (const path of ["/gallery", "/bw-photos", "/color-photos", "/blog", "/blog/missing-post"]) {
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
  await expect(page).toHaveURL(/\/image\//);
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
