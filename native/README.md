# Nurtur — native promo page

A static, one-page promotion site for the Android-first Nurtur app. No build
step, no framework, no external requests. Everything is served from this folder.

## Run it

From this folder:

```sh
cd native
python3 -m http.server 8000
```

Then open <http://localhost:8000>.

Or from the repository root:

```sh
python3 -m http.server 8000
```

Then open <http://localhost:8000/native/>.

## What is here

```text
native/
├── index.html          the page
├── css/style.css       day/night tokens, layout, motion
├── js/main.js          timer demo, theme, waitlist sheet
└── assets/
    ├── fonts/          Fredoka + Nunito Sans (OFL, self-hosted)
    ├── pip-face.svg    the mascot mark
    └── icon-*.png      touch icon and social preview
```

## The interactive bits

- **Theme.** Follows the hour on first load (night after 21:00), remembers a
  manual choice in `localStorage`, and uses a circular View Transition from the
  toggle when the browser supports it. The phone's moon button controls the
  whole page, so you can see the app palette change.
- **Timer demo.** Left and Right start or switch sides, Pause freezes the
  elapsed time, and Done writes a row into Today. The timer is computed from
  timestamps, not a ticking counter, so it keeps running while you scroll.
- **Quick log.** Wet, Dirty, and Both add a row and update the status card. The
  list keeps three rows so the phone mock never overflows.
- **Waitlist sheet.** Opens as a bottom sheet. It tracks the finger 1:1 from the
  grab handle, rubber-bands above the top edge, projects release velocity, hands
  that velocity to a spring, and can be grabbed again mid-flight. Escape,
  backdrop click, and the close button all dismiss it; focus is trapped and
  restored.
- **Reveals.** Sections fade up with an IntersectionObserver. Everything is
  visible with JavaScript disabled, and the whole page respects
  `prefers-reduced-motion`, `prefers-reduced-transparency`, and
  `prefers-contrast`.

## Wire up the waitlist

The form is a demo until you connect it. Set an endpoint before the script
loads:

```html
<script>
  window.NURTUR_WAITLIST_ENDPOINT = "https://example.com/api/beta";
</script>
```

The script posts JSON:

```json
{ "email": "you@example.com", "source": "android-promo" }
```

If the endpoint is empty, the page stores the address in `localStorage` under
`nurtur.promo.beta` and shows the success state. That is useful for a local
prototype, not for collecting real signups.

## Design notes

The palette, fonts, and mascot match the app's design system, so the promo does
not drift from the product. Day is warm cream; night is lifted plum with muted
amber. The primary type is Fredoka; body copy is Nunito Sans.

The page is deliberately calm: no autoplay, no looping background motion, no
countdowns, no fake scarcity. The only ongoing animation is the small status dot
on a running feed.

Prices and feature availability come from [`../NATIVE.md`](../NATIVE.md) and are
labelled as planned for the Android launch.
