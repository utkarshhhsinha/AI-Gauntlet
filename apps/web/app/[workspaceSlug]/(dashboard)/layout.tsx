"use client";

import { Suspense } from "react";
import { DashboardLayout } from "@ai-gauntlet/views/layout";
import { AIGauntletIcon } from "@ai-gauntlet/ui/components/common/ai-gauntlet-icon";
import { SearchCommand, SearchTrigger } from "@ai-gauntlet/views/search";
import { FloatingChat } from "@ai-gauntlet/views/chat";
import { WebNotificationBridge } from "@/components/web-notification-bridge";
import { WorkspaceDocumentTitle } from "@/platform/workspace-document-title";

export default function Layout({ children }: { children: React.ReactNode }) {
  return (
    <>
      {/* useSearchParams requires a Suspense boundary in the app router. Sits
          outside DashboardLayout so the tab is named while the guard is still
          resolving the workspace. */}
      <Suspense fallback={null}>
        <WorkspaceDocumentTitle />
      </Suspense>
      <DashboardLayout
        loadingIndicator={<AIGauntletIcon className="size-6" />}
        searchSlot={<SearchTrigger />}
        extra={
          <>
            <SearchCommand />
            <WebNotificationBridge />
            <FloatingChat />
          </>
        }
      >
        {children}
      </DashboardLayout>
    </>
  );
}
