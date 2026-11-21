"use client";

import { useWorkspaceId } from "@ai-gauntlet/core";
import { useWorkspacePresencePrefetch } from "@ai-gauntlet/core/agents";

// Mount once inside any subtree that's already gated on "workspace resolved"
// (DashboardLayout on web, WorkspaceRouteLayout on desktop). useWorkspaceId
// throws when called outside a resolved workspace — the gating in those
// layouts guarantees this component never sees that state.
export function WorkspacePresencePrefetch() {
  const wsId = useWorkspaceId();
  useWorkspacePresencePrefetch(wsId);
  return null;
}
