/* MARS website. Vanilla JS, no dependencies.
 * Theme toggle, mobile menu, copy button and the faux reader slideshow. */
(function () {
  "use strict";

  var root = document.documentElement;
  var THEME_KEY = "mars-theme";
  var reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)");
  var lightQuery = window.matchMedia("(prefers-color-scheme: light)");

  function readTheme() {
    try {
      var v = localStorage.getItem(THEME_KEY);
      return v === "light" || v === "dark" ? v : null;
    } catch (e) {
      return null;
    }
  }
  function saveTheme(v) {
    try {
      localStorage.setItem(THEME_KEY, v);
    } catch (e) { /* private mode or blocked storage: the choice just is not remembered */ }
  }
  function currentTheme() {
    var forced = root.getAttribute("data-theme");
    if (forced === "light" || forced === "dark") return forced;
    return lightQuery.matches ? "light" : "dark";
  }

  // Apply a stored choice (the inline head script already did this before paint).
  var stored = readTheme();
  if (stored) root.setAttribute("data-theme", stored);

  var themeBtn = document.querySelector("[data-theme-toggle]");
  if (themeBtn) {
    themeBtn.addEventListener("click", function () {
      var next = currentTheme() === "dark" ? "light" : "dark";
      root.setAttribute("data-theme", next);
      saveTheme(next);
    });
  }

  // Mobile menu: one button, aria-expanded, Escape closes and returns focus.
  var menuBtn = document.querySelector("[data-menu-toggle]");
  var menu = document.getElementById("mobile-menu");
  function setMenu(open) {
    if (!menuBtn || !menu) return;
    menuBtn.setAttribute("aria-expanded", open ? "true" : "false");
    menu.hidden = !open;
    document.body.style.overflow = open ? "hidden" : "";
  }
  if (menuBtn && menu) {
    menuBtn.addEventListener("click", function () {
      setMenu(menuBtn.getAttribute("aria-expanded") !== "true");
    });
    document.addEventListener("keydown", function (e) {
      if (e.key === "Escape" && menuBtn.getAttribute("aria-expanded") === "true") {
        setMenu(false);
        menuBtn.focus();
      }
    });
    menu.addEventListener("click", function (e) {
      if (e.target.closest("a")) setMenu(false);
    });
    var wide = window.matchMedia("(min-width: 768px)");
    var onWide = function () { if (wide.matches) setMenu(false); };
    if (wide.addEventListener) wide.addEventListener("change", onWide);
  }

  // Copy button for the one-line Docker command.
  var copyBtn = document.querySelector("[data-copy]");
  if (copyBtn) {
    var label = copyBtn.querySelector("[data-copy-label]");
    var copiedText = copyBtn.getAttribute("data-copied");
    var idleText = label ? label.textContent : "";
    var timer = null;
    var target = document.getElementById(copyBtn.getAttribute("aria-controls"));

    var flash = function () {
      copyBtn.classList.add("is-copied");
      if (label) label.textContent = copiedText;
      clearTimeout(timer);
      timer = setTimeout(function () {
        copyBtn.classList.remove("is-copied");
        if (label) label.textContent = idleText;
      }, 2000);
    };
    // Select the command so the reader can press Ctrl+C / Cmd+C if the clipboard API is refused.
    var fallback = function () {
      if (!target) return;
      try {
        var range = document.createRange();
        range.selectNodeContents(target);
        var sel = window.getSelection();
        sel.removeAllRanges();
        sel.addRange(range);
        if (document.execCommand("copy")) {
          flash();
          sel.removeAllRanges();
        }
      } catch (e) { /* the text stays selected for a manual copy */ }
    };

    copyBtn.addEventListener("click", function () {
      var text = target ? target.textContent.trim() : "";
      if (navigator.clipboard && window.isSecureContext) {
        navigator.clipboard.writeText(text).then(flash, fallback);
      } else {
        fallback();
      }
    });
  }

  // Faux reader: pages slide in from the left (right-to-left reading).
  // Runs only while on screen, pauses on hover, static under reduced motion.
  var stage = document.getElementById("reader-window");
  if (stage) {
    var pages = Array.prototype.slice.call(stage.querySelectorAll("img"));
    var pager = Array.prototype.slice.call(document.querySelectorAll(".pager span"));
    var cur = 0;
    var timer2 = null;
    var inView = false;
    var hovered = false;
    var INTERVAL = 3500;
    var SLIDE = 600;

    var updatePager = function () {
      pager.forEach(function (d, i) { d.classList.toggle("is-on", i === cur); });
    };

    var show = function (next) {
      var prev = pages[cur];
      var inc = pages[next];
      pages.forEach(function (p) { p.style.zIndex = "1"; });
      prev.style.zIndex = "2";
      inc.style.zIndex = "3";
      // Park the incoming page off the left edge without a transition, then let it slide in.
      inc.style.transition = "none";
      inc.style.transform = "translate3d(-100%, 0, 0)";
      void inc.offsetWidth;
      inc.style.transition = "";
      inc.style.transform = "translate3d(0, 0, 0)";
      cur = next;
      updatePager();
      setTimeout(function () {
        if (cur === next) {
          inc.style.zIndex = "2";
          prev.style.zIndex = "1";
        }
      }, SLIDE + 50);
    };

    var tick = function () {
      if (!hovered) show((cur + 1) % pages.length);
    };
    var start = function () {
      if (timer2 || reduceMotion.matches) return;
      timer2 = setInterval(tick, INTERVAL);
    };
    var stop = function () {
      clearInterval(timer2);
      timer2 = null;
    };

    pages.forEach(function (p, i) {
      p.style.zIndex = i === 0 ? "2" : "1";
    });
    updatePager();

    stage.addEventListener("mouseenter", function () { hovered = true; });
    stage.addEventListener("mouseleave", function () { hovered = false; });

    if ("IntersectionObserver" in window && !reduceMotion.matches) {
      var io = new IntersectionObserver(function (entries) {
        inView = entries[0].isIntersecting;
        if (inView) start(); else stop();
      }, { threshold: 0.35 });
      io.observe(stage);
    }
  }
})();
