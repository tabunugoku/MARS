// MARS website: theme toggle, copy buttons and the two quiet moments
// (the tagline's words lighting up, the start steps filling in like the
// app's dot track). The theme itself is set by the inline script in <head>
// before first paint; this file only reacts to the toggle.
(() => {
  const root = document.documentElement;
  const KEY = "mars-site-theme";
  const reduce = matchMedia("(prefers-reduced-motion: reduce)").matches;

  // Screenshots that have a light-theme twin swap with the theme.
  const syncShots = () => {
    const light = root.dataset.theme === "light";
    for (const img of document.querySelectorAll("img[data-light]")) {
      const want = light ? img.dataset.light : img.dataset.dark;
      if (img.getAttribute("src") !== want) img.setAttribute("src", want);
    }
  };
  syncShots();

  const toggle = document.querySelector("[data-theme-toggle]");
  if (toggle) {
    const label = () => {
      const light = root.dataset.theme === "light";
      toggle.setAttribute("aria-pressed", String(!light));
      toggle.setAttribute("aria-label", light ? toggle.dataset.toDark : toggle.dataset.toLight);
      toggle.title = toggle.getAttribute("aria-label");
    };
    label();
    toggle.addEventListener("click", () => {
      root.dataset.theme = root.dataset.theme === "light" ? "dark" : "light";
      try { localStorage.setItem(KEY, root.dataset.theme); } catch { /* private mode */ }
      document.querySelector('meta[name="theme-color"]')?.setAttribute("content", root.dataset.theme === "light" ? "#f8f5f4" : "#120d0b");
      label();
      syncShots();
    });
  }

  // A hairline under the nav once the page has scrolled.
  const nav = document.querySelector(".nav");
  if (nav) {
    const onScroll = () => nav.classList.toggle("is-scrolled", scrollY > 8);
    addEventListener("scroll", onScroll, { passive: true });
    onScroll();
  }

  // Copy buttons next to commands.
  for (const btn of document.querySelectorAll("[data-copy]")) {
    btn.addEventListener("click", async () => {
      const text = document.getElementById(btn.dataset.copy)?.textContent ?? "";
      const status = btn.querySelector(".label");
      try {
        await navigator.clipboard.writeText(text.trim());
        btn.classList.add("is-copied");
        if (status) status.textContent = btn.dataset.done;
        document.getElementById("copy-status").textContent = btn.dataset.done;
        setTimeout(() => {
          btn.classList.remove("is-copied");
          if (status) status.textContent = btn.dataset.idle;
        }, 1800);
      } catch {
        // Clipboard refused (insecure context): select the text instead.
        const pre = document.getElementById(btn.dataset.copy);
        const range = document.createRange();
        range.selectNodeContents(pre);
        const sel = getSelection();
        sel.removeAllRanges();
        sel.addRange(range);
      }
    });
  }

  // Tagline: each word lights up as it crosses the lower third of the
  // screen, in reading order, once.
  const words = [...document.querySelectorAll(".tagline .w")];
  // Start steps: the dots fill one after another when the list is in view.
  const steps = [...document.querySelectorAll(".step")];
  if (reduce || !("IntersectionObserver" in window)) {
    words.forEach((w) => w.classList.add("on"));
    steps.forEach((s) => s.classList.add("is-done"));
    return;
  }
  const wordIO = new IntersectionObserver((entries) => {
    for (const e of entries) {
      if (!e.isIntersecting) continue;
      e.target.classList.add("on");
      wordIO.unobserve(e.target);
    }
  }, { rootMargin: "0px 0px -35% 0px" });
  words.forEach((w) => wordIO.observe(w));

  const list = document.querySelector(".steps");
  if (list) {
    const stepIO = new IntersectionObserver((entries) => {
      if (!entries.some((e) => e.isIntersecting)) return;
      steps.forEach((s, i) => setTimeout(() => s.classList.add("is-done"), 180 * i));
      stepIO.disconnect();
    }, { rootMargin: "0px 0px -25% 0px" });
    stepIO.observe(list);
  }
})();
