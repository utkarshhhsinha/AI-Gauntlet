"use client";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@ai-gauntlet/ui/components/ui/dialog";
import { Button } from "@ai-gauntlet/ui/components/ui/button";
import type { TaskRoutingInfo } from "@ai-gauntlet/core/types";
import { ArrowRight, Bot, Cpu, Gauge, GitFork, ShieldAlert } from "lucide-react";

interface AutoRouteDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  routing?: TaskRoutingInfo | null;
  runTitle?: string;
}

export function AutoRouteDialog({
  open,
  onOpenChange,
  routing,
  runTitle,
}: AutoRouteDialogProps) {
  if (!routing) return null;

  const fromModel = routing.routed_from || "claude-haiku-4.5";
  const toModel = routing.chosen_model || "claude-sonnet-4.5";
  const reason =
    routing.auto_route_reason ||
    (routing.policy_reason
      ? `Auto-routed according to policy: ${routing.policy_reason}`
      : "Model token quota exhausted (100% threshold reached). Cerebra automatically auto-routed to a higher-capacity model in the tier map to prevent execution interruption.");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <div className="flex items-center gap-2 text-rose-600 dark:text-rose-400 font-semibold">
            <GitFork className="h-5 w-5" />
            <DialogTitle>Cerebra Auto-Routing</DialogTitle>
          </div>
          <DialogDescription>
            {runTitle ? `Run: ${runTitle}` : "Model failover and auto-routing details"}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          {/* Model Transition Card */}
          <div className="rounded-lg border bg-muted/40 p-4">
            <div className="text-micro font-medium uppercase tracking-wider text-muted-foreground mb-3">
              Model Transition
            </div>
            <div className="flex items-center justify-between gap-3">
              {/* From Model */}
              <div className="flex-1 rounded-md border border-rose-500/20 bg-rose-500/5 p-2.5 text-center">
                <div className="text-micro text-rose-600 dark:text-rose-400 font-medium mb-1 flex items-center justify-center gap-1">
                  <ShieldAlert className="h-3 w-3" />
                  <span>Routed From</span>
                </div>
                <div className="font-mono text-xs font-semibold text-foreground truncate" title={fromModel}>
                  {fromModel}
                </div>
                <div className="text-[10px] text-muted-foreground mt-0.5">
                  Quota Exhausted
                </div>
              </div>

              <div className="flex shrink-0 items-center justify-center text-muted-foreground">
                <ArrowRight className="h-5 w-5 text-rose-500 animate-pulse" />
              </div>

              {/* To Model */}
              <div className="flex-1 rounded-md border border-emerald-500/20 bg-emerald-500/5 p-2.5 text-center">
                <div className="text-micro text-emerald-600 dark:text-emerald-400 font-medium mb-1 flex items-center justify-center gap-1">
                  <Bot className="h-3 w-3" />
                  <span>Routed To</span>
                </div>
                <div className="font-mono text-xs font-semibold text-foreground truncate" title={toModel}>
                  {toModel}
                </div>
                <div className="text-[10px] text-emerald-600 dark:text-emerald-400 mt-0.5 font-medium">
                  Active Model
                </div>
              </div>
            </div>
          </div>

          {/* Reason Card */}
          <div className="rounded-lg border bg-card p-3.5 space-y-2">
            <div className="flex items-center gap-1.5 text-xs font-medium text-foreground">
              <ShieldAlert className="h-4 w-4 text-rose-500" />
              <span>Routing Reason</span>
            </div>
            <p className="text-xs text-muted-foreground leading-relaxed bg-muted/30 p-2.5 rounded border border-border/50">
              {reason}
            </p>
          </div>

          {/* Metadata Grid */}
          <div className="grid grid-cols-2 gap-2 text-xs">
            {routing.tier && (
              <div className="flex items-center gap-2 rounded border bg-card px-3 py-2">
                <Cpu className="h-3.5 w-3.5 text-muted-foreground" />
                <div className="truncate">
                  <div className="text-[10px] text-muted-foreground">Execution Tier</div>
                  <div className="font-medium capitalize">{routing.tier}</div>
                </div>
              </div>
            )}
            {routing.usage_percentage != null && (
              <div className="flex items-center gap-2 rounded border bg-card px-3 py-2">
                <Gauge className="h-3.5 w-3.5 text-muted-foreground" />
                <div className="truncate">
                  <div className="text-[10px] text-muted-foreground">Quota Utilization</div>
                  <div className="font-medium">{routing.usage_percentage.toFixed(1)}%</div>
                </div>
              </div>
            )}
          </div>
        </div>

        <DialogFooter className="sm:justify-end">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => onOpenChange(false)}
          >
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
