const { test, expect } = require("@playwright/test");

// The smoke server starts with a clean database. Existing installs see this
// one-time prompt after the profile fields were introduced, so the browser
// suite completes it before exercising the dashboard.
test("complete the baby profile", async ({ page }) => {
  const born = new Date();
  born.setMonth(born.getMonth() - 4);
  const iso = [
    born.getFullYear(),
    String(born.getMonth() + 1).padStart(2, "0"),
    String(born.getDate()).padStart(2, "0")
  ].join("-");

  await page.goto("/");
  await expect(page.getByRole("heading", { name: "One more thing" })).toBeVisible();

  await page.locator('input[name="baby_birth_date"]').fill(iso);
  await page.locator('select[name="baby_gender"]').selectOption("girl");
  await page.getByRole("button", { name: "Save and continue" }).click();

  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByRole("heading", { name: "Pip" })).toBeVisible();
  await expect(page.locator(".baby-age")).toContainText("old");
  await expect(page.locator(".baby-age")).toContainText("Girl");
});
