const { test, expect } = require("@playwright/test");

const ALBUM_JPEG_BASE64 =
  "/9j/2wCEAAYEBQYFBAYGBQYHBwYIChAKCgkJChQODwwQFxQYGBcUFhYaHSUfGhsjHBYWICwgIyYnKSopGR8tMC0oMCUoKSgBBwcHCggKEwoKEygaFhooKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKP/AABEIACAAIAMBIgACEQEDEQH/xAGiAAABBQEBAQEBAQAAAAAAAAAAAQIDBAUGBwgJCgsQAAIBAwMCBAMFBQQEAAABfQECAwAEEQUSITFBBhNRYQcicRQygZGhCCNCscEVUtHwJDNicoIJChYXGBkaJSYnKCkqNDU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6g4SFhoeIiYqSk5SVlpeYmZqio6Slpqeoqaqys7S1tre4ubrCw8TFxsfIycrS09TV1tfY2drh4uPk5ebn6Onq8fLz9PX29/j5+gEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoLEQACAQIEBAMEBwUEBAABAncAAQIDEQQFITEGEkFRB2FxEyIygQgUQpGhscEJIzNS8BVictEKFiQ04SXxFxgZGiYnKCkqNTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqCg4SFhoeIiYqSk5SVlpeYmZqio6Slpqeoqaqys7S1tre4ubrCw8TFxsfIycrS09TV1tfY2dri4+Tl5ufo6ery8/T19vf4+fr/2gAMAwEAAhEDEQA/AN4LTwtPC08LXvOR7kJDAtPC08LTwtQ5HTCQwLTwtPC1IFqHI86EiMLUgWnhaeFqHI6YSP/Z";

test.describe("Nurtur smoke path at 390×844", () => {
  test("feed survives reload, switch, and stop", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Pip" })).toBeVisible();
    await openQuickLog(page);

    await page.getByRole("button", { name: "Left", exact: true }).click();
    await expect(page.locator(".timer-label")).toContainText("Nursing Left");

    // The server, not the page, owns the active feed.
    await page.reload();
    await openQuickLog(page);
    await expect(page.locator(".timer-label")).toContainText("Nursing Left");

    await page.getByRole("button", { name: /Switch to Right/ }).click();
    await expect(page.locator(".timer-label")).toContainText("Nursing Right");

    await page.getByRole("button", { name: "Done", exact: true }).click();
    await expect(page.getByText(/Last: Right/)).toBeVisible();
  });

  test("diaper and sleep log in one tap", async ({ page }) => {
    await page.goto("/");
    await openQuickLog(page);

    await page.getByRole("button", { name: "Wet", exact: true }).click();
    await expect(page.getByText("Wet diaper logged.")).toBeVisible();

    await page.getByRole("button", { name: "Dirty", exact: true }).click();
    await expect(page.getByText("Dirty diaper logged.")).toBeVisible();

    await page.getByRole("button", { name: "Both", exact: true }).click();
    await expect(page.getByText("Wet and dirty diaper logged.")).toBeVisible();

    await page.getByRole("button", { name: /Start sleep/ }).click();
    await expect(page.getByText("Pip is asleep")).toBeVisible();
    await page.getByRole("button", { name: /Wake up/ }).click();
    await expect(page.getByText(/Awake\. That was/)).toBeVisible();
  });

  test("history delete restores in place within the undo window", async ({ page }) => {
    await page.goto("/timeline");
    await expect(page.getByRole("heading", { name: "History" })).toBeVisible();
    await page.locator(".timeline-day-head").first().click();

    const diaperRow = page.locator("li.event-diaper").first();
    const rowID = await diaperRow.getAttribute("id");
    await diaperRow.getByRole("button", { name: /^Delete diaper/ }).click();

    await expect(page.getByText("Diaper deleted.")).toBeVisible();
    await expect(page.locator(`#${rowID}`)).toHaveCount(0);

    await page.getByRole("button", { name: "Undo", exact: true }).click();
    await expect(page.getByText("Diaper restored.")).toBeVisible();
    await expect(page.locator(`#${rowID}`)).toBeVisible();
  });

  test("tap a history row opens its edit sheet, and settings save", async ({ page }) => {
    await page.goto("/timeline");
    await page.locator(".timeline-day-head").first().click();
    await page.locator(".timeline-row-main").first().click();
    await expect(page.locator("dialog.sheet[open]")).toBeVisible();
    await page.keyboard.press("Escape");

    await page.goto("/settings");
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
    await page.locator('input[name="night_start"]').fill("20:00");
    await page.locator('input[name="night_end"]').fill("06:00");
    await page.getByRole("button", { name: "Save night window" }).click();
    await expect(page.getByText("Night window saved.")).toBeVisible();
    await expect(page.locator('input[name="night_start"]')).toHaveValue("20:00");

    // Saving the name must refresh the header in place, without a navigation.
    await page.locator('input[name="baby_name"]').fill("Aisyah");
    await page.getByRole("button", { name: "Save name" }).click();
    await expect(page.getByText("Baby name saved.")).toBeVisible();
    await expect(page.locator(".baby-name")).toHaveText("Aisyah");

    // A browser tab is not standalone, so the install hint points at the app.
    await expect(page.getByText(/add Nurtur to your home screen/)).toBeVisible();
  });

  test("history and settings use the bottom nav instead of a back link", async ({ page }) => {
    await page.goto("/timeline");
    await expect(page.getByRole("heading", { name: "History" })).toBeVisible();
    await expect(page.locator(".back-link")).toHaveCount(0);

    await page.getByRole("link", { name: "Settings", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
    await expect(page.locator(".back-link")).toHaveCount(0);
  });

  test("album is a fourth tab that fits a 320px phone in night mode", async ({ page }) => {
    await page.addInitScript(() => {
      try {
        localStorage.setItem("nurtur.theme", "night");
      } catch (e) {}
    });
    await page.setViewportSize({ width: 320, height: 720 });
    await page.goto("/album");

    await expect(page.getByRole("heading", { name: "Album" })).toBeVisible();
    await expect(page.getByText("Snap the first one.")).toBeVisible();
    await expect(page.locator("html")).toHaveAttribute("data-theme", "night");

    const albumTab = page.getByRole("link", { name: "Album", exact: true });
    await expect(albumTab).toHaveAttribute("aria-current", "page");
    await expect(page.getByRole("link", { name: "Home", exact: true })).toBeVisible();
    await expect(page.getByRole("link", { name: "History", exact: true })).toBeVisible();
    await expect(page.getByRole("link", { name: "Settings", exact: true })).toBeVisible();

    // The album owns the screen: the quick-log drawer and its + step away.
    await expect(page.locator("[data-quick-log-toggle]")).toBeHidden();
    await expect(page.locator("details.dock-drawer")).toBeHidden();
    await expect(page.locator(".album-capture")).toBeVisible();
    await expect(page.locator(".album-add-btn")).toBeVisible();
    const columns = await page.locator(".tabbar").evaluate((node) => {
      return getComputedStyle(node).gridTemplateColumns.split(" ").filter(Boolean).length;
    });
    expect(columns).toBe(4);

    const overflows = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
    expect(overflows).toBe(false);
  });

  test("album upload adds a photo and serves its thumbnail", async ({ page }) => {
    const jpeg = Buffer.from(ALBUM_JPEG_BASE64, "base64");
    await page.goto("/album");
    await expect(page.getByText("No photos yet.")).toBeVisible();
    await uploadAlbumJpeg(page, jpeg);

    await expect(page.locator(".album-tile")).toHaveCount(1, { timeout: 15_000 });
    await expect(page.getByText("1 photo")).toBeVisible();
    await expect(page.locator(".album-tile-time")).toHaveText(/\d{1,2}:\d{2}/);
    await expect(page.locator(".album-day-title")).toContainText(/Today|Yesterday/);
    const src = await page.locator(".album-tile img").getAttribute("src");
    expect(src).toMatch(/^\/album\/photos\/\d+\/thumb$/);
    const response = await page.request.get(src);
    expect(response.status()).toBe(200);
    expect(response.headers()["content-type"]).toMatch(/^image\/webp|^image\/jpeg$/);
  });

  test("home opens and closes the quick-log sheet", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Today" })).toBeVisible();

    const drawer = page.locator("details.dock-drawer");
    const quick = page.locator("[data-quick-log-toggle]");
    await expect(drawer).not.toHaveAttribute("open", "");
    await expect(quick).toBeVisible();
    await expect(quick).toHaveAttribute("aria-expanded", "false");

    await quick.click();
    await expect(drawer).toHaveAttribute("open", "");
    await expect(quick).toHaveAttribute("aria-expanded", "true");
    await expect(quick.locator(".icon")).toHaveCSS("transform", /matrix/);
    await expect(page.getByRole("button", { name: "Left", exact: true })).toBeVisible();

    await page.getByRole("button", { name: "Wet", exact: true }).click();
    await expect(page.getByText(/Wet diaper( was just)? logged\./)).toBeVisible();

    await page.locator(".dock-close").click();
    await expect(drawer).not.toHaveAttribute("open", "");
    await expect(quick).toHaveAttribute("aria-expanded", "false");

    // The dimmed background collapses the same sheet.
    await quick.click();
    await expect(drawer).toHaveAttribute("open", "");
    await page.locator(".dock-scrim").click({ position: { x: 10, y: 10 } });
    await expect(drawer).not.toHaveAttribute("open", "");
  });

  test("the quick-log + sits where the album camera does", async ({ page }) => {
    await page.goto("/");
    const quick = page.locator("[data-quick-log-toggle]");
    await expect(quick).toBeVisible();
    const quickBox = await quick.boundingBox();

    await page.getByRole("link", { name: "Album", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Album" })).toBeVisible();
    const camera = page.locator(".album-capture");
    await expect(camera).toBeVisible();
    const cameraBox = await camera.boundingBox();

    expect(Math.abs(quickBox.x - cameraBox.x)).toBeLessThan(1);
    expect(Math.abs(quickBox.y - cameraBox.y)).toBeLessThan(1);
    expect(Math.abs(quickBox.width - cameraBox.width)).toBeLessThan(1);
    expect(Math.abs(quickBox.height - cameraBox.height)).toBeLessThan(1);
  });

  test("quick logging starts closed and closes on tab navigation", async ({ page }) => {
    await page.goto("/");
    const drawer = page.locator("details.dock-drawer");
    const quick = page.locator("[data-quick-log-toggle]");
    await expect(drawer).not.toHaveAttribute("open", "");
    await expect(quick).toHaveAttribute("aria-expanded", "false");

    await quick.click();
    await expect(drawer).toHaveAttribute("open", "");
    await expect(quick).toHaveAttribute("aria-expanded", "true");
    await expect(page.locator(".dock-scrim")).toBeVisible();

    await page.locator(".dock-close").click();
    await expect(drawer).not.toHaveAttribute("open", "");
    await expect(quick).toHaveAttribute("aria-expanded", "false");

    // Leaving Home must not hand the old open-by-default rendering back.
    await page.getByRole("link", { name: "History", exact: true }).click();
    await expect(page.getByRole("heading", { name: "History" })).toBeVisible();
    await expect(page.getByRole("link", { name: "History", exact: true })).toHaveAttribute("aria-current", "page");
    await expect(drawer).not.toHaveAttribute("open", "");

    await page.getByRole("link", { name: "Home", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Today" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Home", exact: true })).toHaveAttribute("aria-current", "page");
    await expect(drawer).not.toHaveAttribute("open", "");

    await page.reload();
    await expect(drawer).not.toHaveAttribute("open", "");
    await expect(quick).toHaveAttribute("aria-expanded", "false");
  });

  test("the quick-log button is only on home", async ({ page }) => {
    const quick = page.locator("[data-quick-log-toggle]");

    await page.goto("/timeline");
    await expect(page.getByRole("heading", { name: "History" })).toBeVisible();
    await expect(quick).toBeHidden();

    await page.goto("/settings");
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
    await expect(quick).toBeHidden();

    await page.goto("/album");
    await expect(page.getByRole("heading", { name: "Album" })).toBeVisible();
    await expect(quick).toBeHidden();

    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Today" })).toBeVisible();
    await expect(quick).toBeVisible();
  });

  test("theme toggle flips on a single click in both directions", async ({ page }) => {
    await page.goto("/");
    await page.evaluate(() => localStorage.setItem("nurtur.theme", "day"));
    await page.reload();

    const html = page.locator("html");
    const toggle = page.locator("[data-theme-toggle]").first();
    await expect(html).toHaveAttribute("data-theme", "day");

    await toggle.click();
    await expect(html).toHaveAttribute("data-theme", "night");

    await toggle.click();
    await expect(html).toHaveAttribute("data-theme", "day");
  });

  test("theme still changes after saving another setting", async ({ page }) => {
    await page.goto("/");
    await page.evaluate(() => localStorage.setItem("nurtur.theme", "day"));
    await page.goto("/settings");

    const html = page.locator("html");
    await expect(html).toHaveAttribute("data-theme", "day");

    // Saving a setting rebuilds the panel, including the Theme card.
    await page.locator('input[name="night_start"]').fill("21:00");
    await page.locator('input[name="night_end"]').fill("07:00");
    await page.getByRole("button", { name: "Save night window" }).click();
    await expect(page.getByText("Night window saved.")).toBeVisible();

    const themeButton = page.locator('section[aria-labelledby="settings-theme"] [data-theme-toggle]');
    await expect(themeButton).toHaveAttribute("aria-label", "Theme: day. Activate for night.");
    await themeButton.click();
    await expect(html).toHaveAttribute("data-theme", "night");
    await expect(themeButton).toHaveAttribute("aria-label", /^Theme: night\./);
  });

  test("nursing alert pings at the threshold and repeats until done", async ({ page }) => {
    // Control Date/timers so the ten-minute threshold can be crossed quickly.
    await page.clock.install();
    await page.goto("/settings");
    await page.locator('input[name="feed_alert_minutes"]').fill("1");
    await page.getByRole("button", { name: "Save alert" }).click();
    await expect(page.getByText("Nursing alert saved.")).toBeVisible();
    await expect(page.locator('input[name="feed_alert_minutes"]')).toHaveValue("1");

    await page.goto("/");
    await openQuickLog(page);
    await page.getByRole("button", { name: "Left", exact: true }).click();
    await expect(page.locator(".timer-label")).toContainText("Nursing Left");

    await page.clock.fastForward(61_000);
    await expect(page.getByText(/Nursing for 1 minute\./)).toBeVisible();

    await page.clock.fastForward(60_000);
    await expect(page.getByText(/Nursing for 2 minutes\./)).toBeVisible();

    await page.getByRole("button", { name: "Done", exact: true }).click();
    await expect(page.locator(".timer-label")).toHaveCount(0);
  });

  test("nursing alert still pings on the settings page", async ({ page }) => {
    await page.clock.install();
    await page.goto("/settings");
    await page.locator('input[name="feed_alert_minutes"]').fill("1");
    await page.getByRole("button", { name: "Save alert" }).click();
    await expect(page.getByText("Nursing alert saved.")).toBeVisible();

    await page.goto("/");
    await openQuickLog(page);
    await page.getByRole("button", { name: "Left", exact: true }).click();
    await expect(page.locator(".timer-label")).toContainText("Nursing Left");

    // Settings has no timer card, so the alert relies on the page-level feed data.
    await page.goto("/settings");
    await page.clock.fastForward(61_000);
    await expect(page.getByText(/Nursing for 1 minute\./)).toBeVisible();

    await page.goto("/");
    await openQuickLog(page);
    await page.getByRole("button", { name: "Done", exact: true }).click();
    await expect(page.locator(".timer-label")).toHaveCount(0);
  });

  test("pause holds the timer until resume", async ({ page }) => {
    await page.goto("/");
    await openQuickLog(page);
    await page.getByRole("button", { name: "Left", exact: true }).click();
    await expect(page.locator(".timer-label")).toContainText("Nursing Left");

    await page.getByRole("button", { name: "Pause nursing" }).click();
    await expect(page.locator(".timer-label")).toContainText("Paused Left");
    await expect(page.getByRole("button", { name: "Resume nursing Left" })).toBeVisible();

    // Pausing is server-owned, so it survives a reload and the digits stop.
    await page.reload();
    await openQuickLog(page);
    await expect(page.locator(".timer-label")).toContainText("Paused Left");
    const frozen = await page.locator("[data-timer-digits]").textContent();
    await page.waitForTimeout(1200);
    await expect(page.locator("[data-timer-digits]")).toHaveText(frozen);

    await page.getByRole("button", { name: "Resume nursing Left" }).click();
    await expect(page.locator(".timer-label")).toContainText("Nursing Left");
    await page.getByRole("button", { name: "Done", exact: true }).click();
    await expect(page.getByText(/Last: Left/)).toBeVisible();
  });

  test("tapping the running activity row opens the quick-log sheet", async ({ page }) => {
    await page.goto("/");
    await openQuickLog(page);
    await page.getByRole("button", { name: /Start sleep/ }).click();
    await expect(page.locator(".sleep-pill.is-asleep")).toBeVisible();
    await closeQuickLog(page);

    // The running activity's row in Today is a shortcut for the + menu.
    const drawer = page.locator("details.dock-drawer");
    const liveRow = page.locator(".feed-row.is-active .live-row-main");
    await expect(liveRow).toHaveAttribute("aria-expanded", "false");
    await liveRow.click();
    await expect(drawer).toHaveAttribute("open", "");
    await expect(liveRow).toHaveAttribute("aria-expanded", "true");
    await expect(page.getByRole("button", { name: /Wake up/ })).toBeVisible();
    await closeQuickLog(page);

    // Status cards keep opening their own detail sheets.
    await page.locator(".status-card.status-sleep").click();
    await expect(
      page.locator("dialog.sheet[open]").getByRole("heading", { name: "Sleep", exact: true })
    ).toBeVisible();

    await page.getByRole("button", { name: "Close", exact: true }).click();
    await openQuickLog(page);
    await page.getByRole("button", { name: /Wake up/ }).click();
    await expect(page.getByText(/Awake\. That was/)).toBeVisible();
  });

  test("quick log focuses on the running activity", async ({ page }) => {
    await page.goto("/");
    const drawer = page.locator("details.dock-drawer");
    await openQuickLog(page);
    await expect(drawer).toHaveAttribute("open", "");

    // With nothing running, every option is available.
    await expect(page.getByRole("button", { name: "Wet", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: /Start sleep/ })).toBeVisible();
    await expect(page.getByRole("button", { name: "Left", exact: true })).toBeVisible();

    // Nursing hides the diaper and sleep options until the feed is done.
    await page.getByRole("button", { name: "Left", exact: true }).click();
    await expect(page.locator(".timer-label")).toContainText("Nursing Left");
    await expect(page.getByRole("button", { name: "Wet", exact: true })).toBeHidden();
    await expect(page.getByRole("button", { name: /Start sleep/ })).toBeHidden();

    await page.getByRole("button", { name: "Done", exact: true }).click();
    await expect(page.getByRole("button", { name: "Wet", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: /Start sleep/ })).toBeVisible();
    await expect(page.getByRole("button", { name: "Left", exact: true })).toBeVisible();

    // Sleep leaves only the wake-up control.
    await page.getByRole("button", { name: /Start sleep/ }).click();
    await expect(page.locator(".sleep-pill.is-asleep")).toBeVisible();
    await expect(page.getByRole("button", { name: "Wet", exact: true })).toBeHidden();
    await expect(page.getByRole("button", { name: "Left", exact: true })).toBeHidden();

    // Waking brings the other options back.
    await page.getByRole("button", { name: /Wake up/ }).click();
    await expect(page.getByRole("button", { name: "Wet", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Left", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: /Start sleep/ })).toBeVisible();
  });

  test("the app shell stays put and only the page body scrolls", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator(".app-main")).toBeVisible();

    const layout = await page.evaluate(() => {
      const app = document.querySelector(".app").getBoundingClientRect();
      const tabbar = document.querySelector(".tabbar").getBoundingClientRect();
      return {
        documentScrollHeight: document.scrollingElement.scrollHeight,
        viewportHeight: window.innerHeight,
        appTop: app.top,
        appBottom: app.bottom,
        tabbarBottom: tabbar.bottom
      };
    });

    // The document is exactly one screen tall: iOS must not be able to drag the
    // header under the status bar or the tab bar off the bottom edge.
    expect(layout.documentScrollHeight).toBeLessThanOrEqual(layout.viewportHeight);
    expect(layout.appTop).toBe(0);
    expect(layout.appBottom).toBe(layout.viewportHeight);
    expect(layout.tabbarBottom).toBe(layout.viewportHeight);
  });

  test("export endpoints download data", async ({ request }) => {
    const csv = await request.get("/export.csv");
    expect(csv.status()).toBe(200);
    expect(csv.headers()["content-disposition"]).toContain("attachment");
    expect(await csv.text()).toContain("kind,id,started_at");

    const json = await request.get("/export.json");
    expect(json.status()).toBe(200);
    expect(await json.text()).toContain('"feeds"');

    const sql = await request.get("/export.sql");
    expect(sql.status()).toBe(200);
    expect(await sql.text()).toContain("BEGIN TRANSACTION");
  });
});

async function openQuickLog(page) {
  const drawer = page.locator("details.dock-drawer");
  if (!(await drawer.evaluate((el) => el.open))) {
    await page.locator("[data-quick-log-toggle]").click();
  }
  await expect(drawer).toHaveAttribute("open", "");
}

async function closeQuickLog(page) {
  const drawer = page.locator("details.dock-drawer");
  if (await drawer.evaluate((el) => el.open)) {
    await page.locator(".dock-close").click();
  }
  await expect(drawer).not.toHaveAttribute("open", "");
}

test.describe("Album capture dates", () => {
  test.use({ timezoneId: "Asia/Kuala_Lumpur" });

  test("EXIF capture time drives the day, with and without an offset", async ({ page }) => {
    const first = albumExifDate(2);
    const second = albumExifDate(3);

    await page.goto("/album");
    await uploadAlbumJpeg(page, Buffer.from(ALBUM_JPEG_BASE64, "base64"));
    await expect(page.locator(".album-tile")).toHaveCount(1, { timeout: 15_000 });

    await uploadAlbumJpeg(page, exifJpeg(Buffer.from(ALBUM_JPEG_BASE64, "base64"), first.exif, ""));
    await expect(page.getByText(/Saved to /)).toBeVisible({ timeout: 15_000 });
    await page.getByRole("link", { name: "View that day" }).click();
    await expect(page.locator(`#album-day-${first.key} .album-tile-time`)).toHaveText("2:07 PM");

    const offsetExif = second.exif.replace("14:07:00", "06:07:00");
    await uploadAlbumJpeg(page, exifJpeg(Buffer.from(ALBUM_JPEG_BASE64, "base64"), offsetExif, "+00:00"));
    await expect(page.getByText(/Saved to /)).toBeVisible({ timeout: 15_000 });
    await page.getByRole("link", { name: "View that day" }).click();
    await expect(page.locator(`#album-day-${second.key} .album-tile-time`)).toHaveText("2:07 PM");
  });

  test("earlier days load in place with their date and time", async ({ page }) => {
    const older = albumExifDate(9);

    await page.goto("/album");
    await uploadAlbumJpeg(
      page,
      exifJpeg(Buffer.from(ALBUM_JPEG_BASE64, "base64"), older.exif, "")
    );
    await expect(page.getByText(/Saved to /)).toBeVisible({ timeout: 15_000 });

    // The old day is outside the first page's seven-day budget, so the paging
    // control is present but the tile is not loaded yet.
    await page.goto("/album");
    const tile = page.locator(`#album-day-${older.key} .album-tile`);
    await expect(page.getByRole("link", { name: "Load earlier days" })).toBeVisible();
    await expect(tile).toHaveCount(0);

    await page.getByRole("link", { name: "Load earlier days" }).click();
    await expect(tile).toHaveCount(1);
    await expect(page.locator(`#album-day-${older.key} .album-tile-time`)).toHaveText("2:07 PM");
    await expect(page.locator(`#album-day-${older.key} .album-day-title`)).toContainText(
      older.heading
    );
    await expect(page.locator("#album-loading")).toBeHidden();
  });
});

async function uploadAlbumJpeg(page, buffer) {
  await page.locator("[data-album-add]").first().click();
  await expect(page.locator("#album-chooser")).toHaveAttribute("open", "");
  const [chooser] = await Promise.all([
    page.waitForEvent("filechooser"),
    page.locator('[data-album-source="library"]').click()
  ]);
  await chooser.setFiles({
    name: "photo.jpg",
    mimeType: "image/jpeg",
    buffer
  });
}

// albumExifDate returns an EXIF date at 14:07 in the household timezone, a few
// whole days ago, plus the local date key the server will use.
function albumExifDate(daysAgo) {
  const date = new Date(Date.now() - daysAgo * 24 * 60 * 60 * 1000);
  const parts = {};
  new Intl.DateTimeFormat("en-GB", {
    timeZone: "Asia/Kuala_Lumpur",
    year: "numeric",
    month: "2-digit",
    day: "2-digit"
  })
    .formatToParts(date)
    .forEach((part) => {
      if (part.type !== "literal") parts[part.type] = part.value;
    });
  return {
    key: `${parts.year}-${parts.month}-${parts.day}`,
    exif: `${parts.year}:${parts.month}:${parts.day} 14:07:00`,
    heading: new Intl.DateTimeFormat("en-US", {
      timeZone: "Asia/Kuala_Lumpur",
      weekday: "long",
      month: "long",
      day: "numeric"
    }).format(date)
  };
}

// exifJpeg inserts a minimal APP1 EXIF block after the JPEG SOI. It carries
// Orientation, DateTimeOriginal, and optionally OffsetTimeOriginal, matching
// the tags album.js reads from a real phone photo.
function exifJpeg(jpeg, dateText, offsetText) {
  const date = Buffer.from(dateText + "\0", "ascii");
  const offset = offsetText ? Buffer.from(offsetText + "\0", "ascii") : null;
  const ifd0Entries = 2;
  const ifd0Size = 2 + ifd0Entries * 12 + 4;
  const exifEntries = offset ? 2 : 1;
  const exifOffset = 8 + ifd0Size;
  const exifSize = 2 + exifEntries * 12 + 4;
  const dateOffset = exifOffset + exifSize;
  const offsetOffset = dateOffset + date.length;
  const tiffLength = offsetOffset + (offset ? offset.length : 0);

  const tiff = Buffer.alloc(tiffLength);
  tiff.write("MM", 0, "ascii");
  tiff.writeUInt16BE(42, 2);
  tiff.writeUInt32BE(8, 4);
  tiff.writeUInt16BE(ifd0Entries, 8);

  let entry = 8 + 2;
  tiff.writeUInt16BE(0x0112, entry);
  tiff.writeUInt16BE(3, entry + 2);
  tiff.writeUInt32BE(1, entry + 4);
  tiff.writeUInt16BE(1, entry + 8);
  entry += 12;
  tiff.writeUInt16BE(0x8769, entry);
  tiff.writeUInt16BE(4, entry + 2);
  tiff.writeUInt32BE(1, entry + 4);
  tiff.writeUInt32BE(exifOffset, entry + 8);
  tiff.writeUInt32BE(0, 8 + 2 + ifd0Entries * 12);

  tiff.writeUInt16BE(exifEntries, exifOffset);
  let exifEntry = exifOffset + 2;
  tiff.writeUInt16BE(0x9003, exifEntry);
  tiff.writeUInt16BE(2, exifEntry + 2);
  tiff.writeUInt32BE(date.length, exifEntry + 4);
  tiff.writeUInt32BE(dateOffset, exifEntry + 8);
  exifEntry += 12;
  if (offset) {
    tiff.writeUInt16BE(0x9011, exifEntry);
    tiff.writeUInt16BE(2, exifEntry + 2);
    tiff.writeUInt32BE(offset.length, exifEntry + 4);
    tiff.writeUInt32BE(offsetOffset, exifEntry + 8);
  }
  tiff.writeUInt32BE(0, exifOffset + 2 + exifEntries * 12);
  date.copy(tiff, dateOffset);
  if (offset) offset.copy(tiff, offsetOffset);

  const app1 = Buffer.alloc(10 + tiff.length);
  app1[0] = 0xff;
  app1[1] = 0xe1;
  app1.writeUInt16BE(2 + 6 + tiff.length, 2);
  app1.write("Exif\0\0", 4, "ascii");
  tiff.copy(app1, 10);
  return Buffer.concat([jpeg.subarray(0, 2), app1, jpeg.subarray(2)]);
}
