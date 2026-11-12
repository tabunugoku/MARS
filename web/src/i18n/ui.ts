import { defineMessages, translate } from "./index";

// Strings the Fluid components (src/components/ui) carry in English upstream.
// scripts/localize-fluid.mjs replaces each one with uiText("key"); this file
// holds both languages. Keep the keys in step with that script's table.
const m = defineMessages({
  ja: {
    close: "閉じる",
    collapseSidebar: "サイドバーを閉じる",
    expandSidebar: "サイドバーを開く",
    toggleSidebar: "サイドバーの表示切り替え",
    resizeSidebar: "サイドバーの幅を変える",
    peekSidebar: "サイドバーを一時表示",
    sidebar: "サイドバー",
    unread: "、未読あり",
    navigation: "ナビゲーション",
    increase: "増やす",
    decrease: "減らす",
  },
  en: {
    close: "Close",
    collapseSidebar: "Collapse sidebar",
    expandSidebar: "Expand sidebar",
    toggleSidebar: "Toggle sidebar",
    resizeSidebar: "Resize or collapse sidebar",
    peekSidebar: "Peek sidebar",
    sidebar: "Sidebar",
    unread: ", unread",
    navigation: "Navigation",
    increase: "Increase",
    decrease: "Decrease",
  },
});

export type UiKey = keyof typeof m.ja;

/** A Fluid component's built-in string, in the current language. */
export const uiText = (key: UiKey): string => translate(m, key);
