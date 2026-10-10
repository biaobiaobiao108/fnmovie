const menuButton = document.querySelector(".menu-toggle");
const siteNav = document.querySelector(".site-nav");

menuButton?.addEventListener("click", () => {
  const isOpen = menuButton.getAttribute("aria-expanded") === "true";
  menuButton.setAttribute("aria-expanded", String(!isOpen));
  menuButton.setAttribute("aria-label", isOpen ? "打开导航菜单" : "关闭导航菜单");
  siteNav?.classList.toggle("is-open", !isOpen);
});

siteNav?.addEventListener("click", (event) => {
  const clickedLink = event.target instanceof Element ? event.target.closest("a") : null;
  if (clickedLink && siteNav.contains(clickedLink)) {
    menuButton?.setAttribute("aria-expanded", "false");
    menuButton?.setAttribute("aria-label", "打开导航菜单");
    siteNav.classList.remove("is-open");
  }
});
