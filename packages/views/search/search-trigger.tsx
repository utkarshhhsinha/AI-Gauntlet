"use client";

import { Search } from "lucide-react";
import { SidebarMenuButton } from "@ai-gauntlet/ui/components/ui/sidebar";
import {
  useShortcut,
} from "@ai-gauntlet/core/shortcuts";
import { useSearchStore } from "./search-store";
import { useT } from "../i18n";
import { ShortcutKeycaps } from "../common/shortcut-keycaps";

export function SearchTrigger() {
  const { t } = useT("search");
  const shortcut = useShortcut("openSearch");
  return (
    <SidebarMenuButton
      className="h-8 w-full rounded-lg border border-sidebar-border/60 bg-muted/30 px-2.5 text-xs text-muted-foreground shadow-2xs transition-all duration-150 hover:border-sidebar-border hover:bg-muted/60 hover:text-foreground group cursor-pointer"
      onClick={() => useSearchStore.getState().setOpen(true)}
    >
      <Search className="h-3.5 w-3.5 text-muted-foreground/80 group-hover:text-foreground transition-colors" />
      <span className="font-normal text-muted-foreground/90 group-hover:text-foreground transition-colors">{t(($) => $.trigger.label)}</span>
      {shortcut ? (
        <ShortcutKeycaps
          shortcut={shortcut}
          decorative
          className="pointer-events-none ml-auto border-border/50 bg-background/50 text-[10px] opacity-75 group-hover:opacity-100 transition-opacity"
        />
      ) : null}
    </SidebarMenuButton>
  );
}
