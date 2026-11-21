import { useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { AutopilotDetailPage as AutopilotDetail } from "@ai-gauntlet/views/autopilots/components";
import { useWorkspaceId } from "@ai-gauntlet/core/hooks";
import { autopilotDetailOptions } from "@ai-gauntlet/core/autopilots/queries";
import { useDocumentTitle } from "@/hooks/use-document-title";

export function AutopilotDetailPage() {
  const { id } = useParams<{ id: string }>();
  const wsId = useWorkspaceId();
  const { data } = useQuery(autopilotDetailOptions(wsId, id!));

  // Plain text only — no leading ⚡ glyph in the title (MUL-4370).
  useDocumentTitle(data ? data.autopilot.title : "Autopilot");

  if (!id) return null;
  return <AutopilotDetail autopilotId={id} />;
}
