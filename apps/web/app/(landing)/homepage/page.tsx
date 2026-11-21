import type { Metadata } from "next";
import { AIGauntletLanding } from "@/features/landing/components/ai-gauntlet-landing";

export const metadata: Metadata = {
  title: "Homepage",
  description:
    "AI Gauntlet — open-source platform that turns coding agents into real teammates. Assign tasks, track progress, compound skills.",
  openGraph: {
    title: "AI Gauntlet — Project Management for Human + Agent Teams",
    description:
      "Manage your human + agent workforce in one place.",
    url: "/homepage",
  },
  alternates: {
    canonical: "/homepage",
  },
};

export default function HomepagePage() {
  return <AIGauntletLanding />;
}
