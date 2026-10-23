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
