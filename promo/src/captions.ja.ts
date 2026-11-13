// Japanese captions: the original text. captions.en.ts translates it.
// One caption per scene; keys match SCENES in Promo.tsx.
import type { Captions } from "./captions";

export const ja: Captions = {
  folders: "マンガのフォルダを、MARS が読み込みます",
  series: "シリーズと巻に、自動でまとまります",
  reader: "ブラウザで、そのまま読めます",
  direction: "右から左も、縦スクロールも",
  resume: "続きは、読んだページから",
  phone: "スマホでは、ホーム画面に追加できます",
  apps: "Mihon や OPDS のアプリからも読めます",
  docker: "Docker のコマンド 1 行で起動します",
  ui: {
    subtitle: "A Reading Server",
    readingDirection: "読み方向",
    modes: ["右→左", "左→右", "縦", "縦スクロール"],
    browser: "ブラウザ",
    browserNote: "PWA としてホーム画面へ",
    mihonNote: "Komga 互換 API",
    opdsNote: "Panels・Chunky・KyBook",
    open: "ブラウザで開く",
  },
};
