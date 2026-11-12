import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import './styles/tokens.css'
import './styles/glow.css'
import App from './App.tsx'

// The service worker (public/sw.js) caches the app shell for the installed
// app. Production builds only: in dev it would fight Vite's module reloads.
// Browsers expose it only on HTTPS or localhost; plain-HTTP LAN access just
// runs without it.
if (import.meta.env.PROD && import.meta.env.MODE !== "mock" && "serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js", { scope: "/" }).catch((err: unknown) => console.warn("[sw]", err));
  });
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
