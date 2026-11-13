export type Lang = "en" | "ja";

export type Captions = {
  folders: string;
  series: string;
  reader: string;
  direction: string;
  resume: string;
  phone: string;
  apps: string;
  docker: string;
  ui: {
    subtitle: string;
    readingDirection: string;
    modes: [string, string, string, string];
    browser: string;
    browserNote: string;
    mihonNote: string;
    opdsNote: string;
    open: string;
  };
};

// The tagline is always English, in both videos.
export const TAGLINE = ["Your", "manga.", "Your", "server.", "Your", "universe."];
export const REPO = "github.com/tabunugoku/MARS";
export const COMMAND = "docker run -d --name mars -p 25600:25600 -v mars-data:/data -v /path/to/comics:/comics:ro ghcr.io/tabunugoku/mars:1";
