import { describe, expect, it } from "vitest";
import { workspaceUrlHost } from "./workspace-url";

describe("workspaceUrlHost", () => {
  it("returns the host of a full app URL", () => {
    expect(workspaceUrlHost("https://ai-gauntlet.example.com")).toBe(
      "ai-gauntlet.example.com",
    );
  });

  it("ignores scheme, path, and trailing slash", () => {
    expect(workspaceUrlHost("https://ai-gauntlet.example.com/")).toBe(
      "ai-gauntlet.example.com",
    );
    expect(workspaceUrlHost("http://ai-gauntlet.example.com/app/onboarding")).toBe(
      "ai-gauntlet.example.com",
    );
  });

  it("preserves a non-default port", () => {
    expect(workspaceUrlHost("https://my.host:3000")).toBe("my.host:3000");
  });

  it("accepts a bare host without a scheme", () => {
    expect(workspaceUrlHost("ai-gauntlet.example.com")).toBe("ai-gauntlet.example.com");
    expect(workspaceUrlHost("ai-gauntlet.example.com/path")).toBe(
      "ai-gauntlet.example.com",
    );
  });

  it("falls back to the brand host when no app URL is configured", () => {
    expect(workspaceUrlHost("")).toBe("ai-gauntlet.org");
    expect(workspaceUrlHost("   ")).toBe("ai-gauntlet.org");
    expect(workspaceUrlHost(null)).toBe("ai-gauntlet.org");
    expect(workspaceUrlHost(undefined)).toBe("ai-gauntlet.org");
  });
});
