For a solo-user, mother-focused baby tracker built on **Go + SQLite + HTMX**, the golden rule is **zero-friction one-handed use**. She will frequently be half-asleep, holding a nursing or crying baby in one arm, and operating her phone with a single thumb.

---

### 1. Dual-Side Breastfeeding Timer & Logger (Core Feature)

* **One-tap Left / Right toggle:** Tapping "Left" starts the timer for the left breast; tapping "Right" automatically pauses/stops the left and starts the right without needing multiple clicks.
* **Resilient timer state:** Store the timer's `started_at` and `side` in `localStorage` (via lightweight inline JavaScript) and/or sync it to the backend via HTMX so that if the browser refreshes, switches tabs, or her screen locks, the timer doesn't reset or lose counted seconds.
* **Quick-fix / Manual edit:** New parents regularly forget to start or stop the timer. A 1-click modal or inline HTMX form to adjust duration (e.g., "+5 min", "-5 min", or edit start/end times manually) is essential.
* **"Last nursed" indicator:** A high-contrast header badge showing: *"Last side: Left (1 hr 45 min ago)"*. This eliminates the perpetual *"Which side did I feed on last?"* guesswork.

---

### 2. Streamlined Diaper Tracker (Poop & Pee)

* **Single-tap logging:** 3 big, tap-friendly buttons: **Wet**, **Dirty**, and **Both**.
* **Quick details (optional dropdown/chips):** Color (yellow, mustard, green, dark) and consistency (seedy, liquid, hard) — pediatrician check-ups often ask for these details in the early weeks.
* **Time badge:** Displays time elapsed since the last wet or dirty diaper (critical for monitoring hydration and stool frequency).

---

### 3. "Time Elapsed Since" Dashboard (At-a-Glance Status)

Instead of forcing her to read a full log table to figure out what comes next, have the top of the mobile screen display compact status cards:

* **Nursing:** Side + elapsed time (e.g., *Right side · 2h 10m ago*)
* **Diaper:** Type + elapsed time (e.g., *Dirty · 1h 40m ago*)
* **Sleep (if tracked):** Woke up / fell asleep + elapsed time (e.g., *Awake · 45m*)

---

### 4. Sleep & Nap Tracker

* **Simple Start/Stop or Retroactive Log:** Single button to track wake windows and total daytime vs. nighttime sleep duration.
* **Wake Window Counter:** Shows how long baby has been awake, helping anticipate over-tired meltdowns before they happen.

---

### 5. Night Mode & Single-Hand UX

* **Low-Glare Night Mode:** Nursing sessions at 3:00 AM use a lifted plum-charcoal palette with soft off-white text and muted amber accents — no pure black, no glare, and no muddy color blocks.
* **Thumb-zone layout:** Position the primary action buttons (Start/Stop, Diaper log) in the bottom third of the screen within easy reach of one thumb.
* **HTMX swap without page jump:** Use `hx-swap="outerHTML"` or targeted out-of-band updates (`hx-swap-oob="true"`) to update the feed and summary widgets smoothly without losing the user's scroll position.

---

### 6. Chronological Feed with Quick Undo

* **Reverse chronological timeline:** A simple list of the day’s events grouped by date.
* **Undo / Delete button:** Accidental taps happen constantly when nursing. A quick 1-click delete with an HTMX `hx-delete` keeps data clean without requiring tedious confirmation dialogs.

---

### Recommended Minimal SQLite Schema

```sql
CREATE TABLE feeds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at DATETIME NOT NULL,
    ended_at DATETIME,
    left_duration_seconds INTEGER DEFAULT 0,
    right_duration_seconds INTEGER DEFAULT 0,
    last_side TEXT CHECK(last_side IN ('left', 'right')),
    notes TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE diapers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    logged_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    type TEXT NOT NULL CHECK(type IN ('wet', 'dirty', 'both')),
    color TEXT,
    notes TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE sleeps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at DATETIME NOT NULL,
    ended_at DATETIME,
    notes TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

```
