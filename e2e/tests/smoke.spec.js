const { test, expect } = require("@playwright/test");

test.describe("Nurtur smoke path at 390×844", () => {
  test("feed survives reload, switch, and stop", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Pip" })).toBeVisible();

    await page.getByRole("button", { name: "Left", exact: true }).click();
    await expect(page.locator(".timer-label")).toContainText("Nursing Left");

    // The server, not the page, owns the active feed.
    await page.reload();
    await expect(page.locator(".timer-label")).toContainText("Nursing Left");

    await page.getByRole("button", { name: /Switch to Right/ }).click();
    await expect(page.locator(".timer-label")).toContainText("Nursing Right");

    await page.getByRole("button", { name: "Done", exact: true }).click();
    await expect(page.getByText(/Last: Right/)).toBeVisible();
  });

  test("diaper and sleep log in one tap", async ({ page }) => {
    await page.goto("/");

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

  test("history collapses quick logging into an expandable drawer", async ({ page }) => {
    await page.goto("/timeline");
    await expect(page.getByRole("heading", { name: "History" })).toBeVisible();

    const drawer = page.locator("details.dock-drawer");
    const toggle = page.locator(".dock-toggle");
    await expect(drawer).not.toHaveAttribute("open", "");
    await expect(toggle).toBeVisible();

    await toggle.click();
    await expect(drawer).toHaveAttribute("open", "");
    await expect(page.getByRole("button", { name: "Left", exact: true })).toBeVisible();

    await page.getByRole("button", { name: "Wet", exact: true }).click();
    await expect(page.getByText(/Wet diaper( was just)? logged\./)).toBeVisible();

    await toggle.click();
    await expect(drawer).not.toHaveAttribute("open", "");
  });

  test("hiding quick logging sticks on the main screen and reloads", async ({ page }) => {
    await page.goto("/");
    const drawer = page.locator("details.dock-drawer");
    const toggle = page.locator(".dock-toggle");
    await expect(drawer).toHaveAttribute("open", "");

    await toggle.click();
    await expect(drawer).not.toHaveAttribute("open", "");

    // Leaving History must not hand the old open-by-default rendering back.
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

  test("nursing alert pings at the threshold and repeats until done", async ({ page }) => {
    // Control Date/timers so the ten-minute threshold can be crossed quickly.
    await page.clock.install();
    await page.goto("/settings");
    await page.locator('input[name="feed_alert_minutes"]').fill("1");
    await page.getByRole("button", { name: "Save alert" }).click();
    await expect(page.getByText("Nursing alert saved.")).toBeVisible();
    await expect(page.locator('input[name="feed_alert_minutes"]')).toHaveValue("1");

    await page.goto("/");
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
    await page.getByRole("button", { name: "Left", exact: true }).click();
    await expect(page.locator(".timer-label")).toContainText("Nursing Left");

    // Settings has no timer card, so the alert relies on the page-level feed data.
    await page.goto("/settings");
    await page.clock.fastForward(61_000);
    await expect(page.getByText(/Nursing for 1 minute\./)).toBeVisible();

    await page.goto("/");
    await page.getByRole("button", { name: "Done", exact: true }).click();
    await expect(page.locator(".timer-label")).toHaveCount(0);
  });

  test("pause holds the timer until resume", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("button", { name: "Left", exact: true }).click();
    await expect(page.locator(".timer-label")).toContainText("Nursing Left");

    await page.getByRole("button", { name: "Pause nursing" }).click();
    await expect(page.locator(".timer-label")).toContainText("Paused Left");
    await expect(page.getByRole("button", { name: "Resume nursing Left" })).toBeVisible();

    // Pausing is server-owned, so it survives a reload and the digits stop.
    await page.reload();
    await expect(page.locator(".timer-label")).toContainText("Paused Left");
    const frozen = await page.locator("[data-timer-digits]").textContent();
    await page.waitForTimeout(1200);
    await expect(page.locator("[data-timer-digits]")).toHaveText(frozen);

    await page.getByRole("button", { name: "Resume nursing Left" }).click();
    await expect(page.locator(".timer-label")).toContainText("Nursing Left");
    await page.getByRole("button", { name: "Done", exact: true }).click();
    await expect(page.getByText(/Last: Left/)).toBeVisible();
  });

  test("tapping the running activity row opens the quick-log dock", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("button", { name: /Start sleep/ }).click();
    await expect(page.locator(".sleep-pill.is-asleep")).toBeVisible();

    const drawer = page.locator("details.dock-drawer");
    const toggle = page.locator(".dock-toggle");
    await toggle.click();
    await expect(drawer).not.toHaveAttribute("open", "");

    // The running activity's row in Today is a shortcut for the + menu.
    const liveRow = page.locator(".feed-row.is-active .live-row-main");
    await expect(liveRow).toHaveAttribute("aria-expanded", "false");
    await liveRow.click();
    await expect(drawer).toHaveAttribute("open", "");
    await expect(liveRow).toHaveAttribute("aria-expanded", "true");
    await expect(page.getByRole("button", { name: /Wake up/ })).toBeVisible();

    // Status cards keep opening their own detail sheets.
    await page.locator(".status-card.status-sleep").click();
    await expect(
      page.locator("dialog.sheet[open]").getByRole("heading", { name: "Sleep", exact: true })
    ).toBeVisible();

    await page.getByRole("button", { name: "Close", exact: true }).click();
    await page.getByRole("button", { name: /Wake up/ }).click();
    await expect(page.getByText(/Awake\. That was/)).toBeVisible();
  });

  test("quick log focuses on the running activity", async ({ page }) => {
    await page.goto("/");
    const drawer = page.locator("details.dock-drawer");
    if (!(await drawer.evaluate((el) => el.open))) {
      await page.locator(".dock-toggle").click();
    }
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
