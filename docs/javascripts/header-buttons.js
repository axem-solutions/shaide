(function () {
  function formatCount(n) {
    if (n >= 1000) return (n / 1000).toFixed(1).replace(/\.0$/, "") + "k";
    return String(n);
  }

  var ARROW_SVG = '<svg class="hdr-btn__arrow" xmlns="http://www.w3.org/2000/svg" width="7.392" height="12.33" viewBox="0 0 7.392 12.33" fill="none"><path d="M 7.392 4.93 L 4.93 4.93 L 4.93 7.4 L 7.392 7.4 Z M 4.927 2.469 L 2.465 2.469 L 2.465 4.937 L 4.927 4.937 L 4.927 2.47 Z M 2.462 0 L 0 0 L 0 2.469 L 2.462 2.469 Z M 4.927 7.4 L 2.465 7.4 L 2.465 9.868 L 4.927 9.868 Z M 2.462 9.861 L 0 9.861 L 0 12.33 L 2.462 12.33 Z" fill="currentColor"/></svg>';

  function makeButton(opts) {
    var a = document.createElement("a");
    a.href = opts.href;
    a.target = "_blank";
    a.rel = "noopener";
    a.title = opts.title;
    a.className = "hdr-btn" + (opts.outline ? " hdr-btn--outline" : "");
    a.innerHTML = '<span class="hdr-btn__label">' + opts.label + '</span>' + ARROW_SVG;
    return a;
  }

  function syncWidths(buttons) {
    buttons.forEach(function (b) { b.style.minWidth = ""; });
    var max = 0;
    buttons.forEach(function (b) { max = Math.max(max, b.getBoundingClientRect().width); });
    buttons.forEach(function (b) { b.style.minWidth = max + "px"; });
  }

  function setupSearchHint() {
    var input = document.querySelector(".md-search__input");
    if (!input || input.dataset.enhanced) return;
    input.dataset.enhanced = "true";
    input.placeholder = "Search...";

    var isMac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
    document.addEventListener("keydown", function (e) {
      var mod = isMac ? e.metaKey : e.ctrlKey;
      if (mod && e.key.toLowerCase() === "k") {
        e.preventDefault();
        var toggle = document.getElementById("__search");
        if (toggle) toggle.checked = true;
        var box = document.querySelector(".md-search__input");
        if (box) box.focus();
      }
    });
  }

  var COPY_ICON_SVG = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><rect x="9" y="9" width="12" height="12" rx="1" stroke="currentColor" stroke-width="2"/><path d="M5 15H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v1" stroke="currentColor" stroke-width="2"/></svg>';
  var CHECK_ICON_SVG = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><path d="M5 13l4 4L19 7" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>';
  var EXTERNAL_ICON_SVG = '<svg class="page-toolbar__menu-arrow" width="12" height="12" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><path d="M7 17 17 7M9 7h8v8" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>';

  var AI_SERVICES = [
    { key: "claude", label: "Claude", url: "https://claude.ai/new?q=" },
    { key: "chatgpt", label: "ChatGPT", url: "https://chatgpt.com/?q=" },
    { key: "perplexity", label: "Perplexity", url: "https://www.perplexity.ai/search?q=" },
    { key: "grok", label: "Grok", url: "https://grok.com/?q=" },
  ];

  function relativeRootPrefix() {
    var depth = window.location.pathname.replace(/^\/|\/$/g, "").split("/").filter(Boolean).length;
    return depth > 0 ? new Array(depth + 1).join("../") : "./";
  }

  function markdownSourceUrl() {
    var meta = document.querySelector('meta[name="doc-source"]');
    if (!meta) return null;
    return relativeRootPrefix() + "_sources/" + meta.content;
  }

  function stripFrontMatter(text) {
    return text.replace(/^---\r?\n[\s\S]*?\r?\n---\r?\n?/, "");
  }

  function setupCopyButton() {
    var article = document.querySelector(".md-content__inner");
    var h1 = article ? article.querySelector("h1") : null;
    if (!h1 || !article || document.querySelector(".page-title-row")) return;

    var fallbackText = article.innerText;
    var sourceUrl = markdownSourceUrl();
    var pageUrl = "https://www.axem.dev/docs" + window.location.pathname;
    var prompt = "Read " + pageUrl + " so I can ask questions about it.";

    var titleRow = document.createElement("div");
    titleRow.className = "page-title-row";
    h1.insertAdjacentElement("beforebegin", titleRow);
    titleRow.appendChild(h1);

    var actions = document.createElement("div");
    actions.className = "page-toolbar";
    actions.innerHTML =
      '<button type="button" class="page-toolbar__copy" title="Copy for LLM">' + COPY_ICON_SVG + '</button>' +
      '<div class="page-toolbar__open">' +
      '<button type="button" class="page-toolbar__open-trigger"><span class="hdr-btn__label">Open</span>' + ARROW_SVG + '</button>' +
      '<div class="page-toolbar__menu">' +
      AI_SERVICES.map(function (svc) {
        return '<a href="' + svc.url + encodeURIComponent(prompt) + '" target="_blank" rel="noopener">' +
          '<span>' + svc.label + '</span>' + EXTERNAL_ICON_SVG + '</a>';
      }).join("") +
      '</div></div>';
    titleRow.appendChild(actions);

    var line = document.createElement("div");
    line.className = "page-title-row__line";
    titleRow.insertAdjacentElement("afterend", line);

    var openWrap = actions.querySelector(".page-toolbar__open");
    var openTrigger = actions.querySelector(".page-toolbar__open-trigger");
    openTrigger.addEventListener("click", function (e) {
      e.stopPropagation();
      openWrap.classList.toggle("is-open");
    });
    document.addEventListener("click", function (e) {
      if (openWrap.classList.contains("is-open") && !openWrap.contains(e.target)) {
        openWrap.classList.remove("is-open");
      }
    });

    var btn = actions.querySelector(".page-toolbar__copy");
    btn.addEventListener("click", function () {
      var textPromise = sourceUrl
        ? fetch(sourceUrl)
            .then(function (res) { return res.ok ? res.text() : Promise.reject(); })
            .then(stripFrontMatter)
            .catch(function () { return fallbackText; })
        : Promise.resolve(fallbackText);

      textPromise.then(function (text) {
        return navigator.clipboard.writeText(text);
      }).then(function () {
        btn.innerHTML = CHECK_ICON_SVG;
        btn.classList.add("page-toolbar__copy--done");
        setTimeout(function () {
          btn.innerHTML = COPY_ICON_SVG;
          btn.classList.remove("page-toolbar__copy--done");
        }, 1500);
      });
    });
  }

  function setupBrandLockup() {
    var logo = document.querySelector(".md-header__button.md-logo");
    if (!logo || logo.dataset.enhanced) return;
    logo.dataset.enhanced = "true";
    // Leave the href alone: Material already points the logo at the
    // documentation home page.
  }

  var FOOTER_LINKS = [
    { label: "About", url: "https://www.axem.dev/about" },
    { label: "LLM Box", url: "https://www.axem.dev/llm-box" },
    { label: "Agents", url: "https://www.axem.dev/shaide-code" },
    { label: "Blog", url: "https://www.axem.dev/blog" },
    { label: "Careers", url: "https://www.axem.dev/careers" },
    { label: "Contact", url: "https://www.axem.dev/contact" },
  ];

  function setupFooter() {
    var copyright = document.querySelector(".md-footer-meta .md-copyright");
    if (!copyright || copyright.dataset.enhanced) return;
    copyright.dataset.enhanced = "true";
    copyright.classList.add("footer-custom");

    copyright.innerHTML =
      '<span class="footer-custom__copyright">© 2026 axem</span>' +
      '<nav class="footer-custom__links">' +
      FOOTER_LINKS.map(function (l) { return '<a href="' + l.url + '">' + l.label + '</a>'; }).join("") +
      '<a href="https://www.axem.dev/privacy-and-cookie-policy" class="footer-custom__privacy">Privacy Policy</a>' +
      '</nav>';
  }

  function setup() {
    var header = document.querySelector(".md-header__inner");
    setupCopyButton();
    setupBrandLockup();
    setupFooter();
    if (!header || document.querySelector(".hdr-btn-group")) return;

    setupSearchHint();

    var group = document.createElement("div");
    group.className = "hdr-btn-group";

    var gh = makeButton({
      href: "https://github.com/axem-solutions/shaide",
      title: "View on GitHub",
      label: "GitHub",
    });
    var labelEl = gh.querySelector(".hdr-btn__label");
    labelEl.textContent = "GitHub";

    var discord = makeButton({
      href: "https://discord.gg/Sz8urtbUcp",
      title: "Join our Community",
      label: "Community",
      outline: true,
    });

    group.appendChild(gh);
    group.appendChild(discord);

    var oldSource = header.querySelector(".md-header__source");
    if (oldSource) {
      oldSource.replaceWith(group);
    } else {
      header.appendChild(group);
    }

    syncWidths([gh, discord]);

    fetch("https://api.github.com/repos/axem-solutions/shaide")
      .then(function (res) { return res.ok ? res.json() : null; })
      .then(function (data) {
        if (data && typeof data.stargazers_count === "number") {
          labelEl.textContent = "GitHub ★ " + formatCount(data.stargazers_count);
          syncWidths([gh, discord]);
        }
      })
      .catch(function () {});
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", setup);
  } else {
    setup();
  }
  if (window.document$) {
    window.document$.subscribe(setup);
  }
})();
