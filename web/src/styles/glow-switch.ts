// Experiment switch for the glow layer (styles/glow.css). On by default on
// this branch; compare with the flat design via ?glow=0 / ?glow=1 or Alt+G.
// The choice is remembered per browser.
const KEY = "cn-glow";
const root = document.documentElement;

function apply(on: boolean) {
  root.classList.toggle("cn-glow", on);
  localStorage.setItem(KEY, on ? "1" : "0");
}

const param = new URLSearchParams(location.search).get("glow");
apply(param !== null ? param !== "0" : localStorage.getItem(KEY) !== "0");

window.addEventListener("keydown", (e) => {
  if (e.altKey && e.code === "KeyG") {
    e.preventDefault();
    apply(!root.classList.contains("cn-glow"));
  }
});
