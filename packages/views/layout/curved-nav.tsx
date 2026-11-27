"use client";

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
  type WheelEvent,
} from "react";
import type { LucideIcon } from "lucide-react";
import { cn } from "@ai-gauntlet/ui/lib/utils";
import { AppLink } from "../navigation";

/**
 * Curved "wheel" navigation for the sidebar.
 *
 * Every destination is a glass bubble that rides a circular arc on the LEFT
 * edge of the sidebar. The wheel stretches to fill whatever height the sidebar
 * gives it: spacing, the number of visible items and the arc's radius are all
 * derived from the measured height, so a tall window shows most of the list
 * along one long, gentle curve while a short one tightens up.
 *
 * The focused item sits at the vertical centre, nearest the arc's apex and full
 * size; its neighbours slide away along the curve, shrinking and fading. The
 * current route is filled with purple.
 *
 * Focus (what is centred) is separate from the active route, so the wheel can
 * be browsed without navigating:
 *   - mouse wheel / trackpad scroll over the nav steps the focus,
 *   - ArrowUp / ArrowDown move keyboard focus to the neighbouring link,
 *   - tabbing to a link (or clicking one) centres it,
 *   - a route change re-centres the wheel on the new active item.
 *
 * Motion is a critically damped spring on a fractional "position" that is
 * written straight to each element's transform every frame (no React renders
 * per frame), so items stay exactly on the arc mid-flight. Reduced-motion
 * users get an instant snap instead.
 */

export interface CurvedNavItem {
  key: string;
  href: string;
  label: string;
  Icon: LucideIcon;
  active: boolean;
  badge?: ReactNode;
}

const BUBBLE = 48; // bubble diameter
const X_CENTER = 78; // left edge of the focused bubble (the arc's apex)
const GAP = 30; // distance from the guide line to a bubble
const EDGE_PAD = 4; // the farthest bubble keeps this much clearance from the left edge
const MIN_PITCH = 54;
const MAX_PITCH = 80;
const DEFAULT_H = 352; // used until the real height is measured (SSR, tests)

interface Geom {
  h: number; // available height
  pitch: number; // vertical distance between neighbouring items
  half: number; // items shown on each side of the focused one
  radius: number; // arc radius
}

function makeGeom(h: number, n: number): Geom {
  // Spread the list over the full height, within comfortable spacing limits.
  const pitch = Math.min(MAX_PITCH, Math.max(MIN_PITCH, h / Math.max(n, 1)));
  const reach = Math.max(h / 2 - BUBBLE / 2, 60);
  const half = Math.max(2, Math.floor(reach / pitch));
  // Circle through the apex whose sideways pull at the farthest visible item
  // exactly uses the left-hand room: R = (dy² + b²) / 2b.
  const b = X_CENTER - EDGE_PAD;
  const radius = (reach * reach + b * b) / (2 * b);
  return { h, pitch, half, radius };
}

function arcOffset(dy: number, radius: number) {
  const c = Math.sign(dy) * Math.min(Math.abs(dy), radius * 0.95);
  return radius - Math.sqrt(radius * radius - c * c);
}

function layoutFor(d: number, g: Geom) {
  const ad = Math.abs(d);
  const reachItems = g.half + 0.6;
  const dy = d * g.pitch;
  return {
    x: X_CENTER - arcOffset(dy, g.radius),
    y: g.h / 2 + dy - BUBBLE / 2,
    scale: Math.max(0.72, 1 - ad * 0.045),
    opacity: ad >= reachItems ? 0 : Math.max(0, 1 - Math.pow(ad / reachItems, 1.4)),
  };
}

function arcPath(g: Geom) {
  const pts: string[] = [];
  for (let y = 0; y <= g.h; y += 8) {
    const x = X_CENTER - GAP - arcOffset(y - g.h / 2, g.radius);
    pts.push(`${pts.length ? "L" : "M"}${x.toFixed(1)} ${y}`);
  }
  return pts.join(" ");
}

const useIsoLayoutEffect = typeof window !== "undefined" ? useLayoutEffect : useEffect;

export function CurvedNav({
  items,
  "aria-label": ariaLabel,
}: {
  items: CurvedNavItem[];
  "aria-label"?: string;
}) {
  const activeIndex = items.findIndex((i) => i.active);
  const [focus, setFocus] = useState(Math.max(activeIndex, 0));

  const wrapRef = useRef<HTMLDivElement>(null);
  const [h, setH] = useState(DEFAULT_H);
  const itemRefs = useRef<(HTMLAnchorElement | null)[]>([]);
  const posRef = useRef(focus);
  const velRef = useRef(0);
  const wheelAcc = useRef(0);
  const lastWheelStep = useRef(0);

  // A route change re-centres the wheel on the new active item.
  useEffect(() => {
    if (activeIndex >= 0) setFocus(activeIndex);
  }, [activeIndex]);

  // Track the height the sidebar gives us and re-derive the geometry from it.
  useIsoLayoutEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const measure = () => setH(Math.round(el.clientHeight) || DEFAULT_H);
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const geom = useMemo(() => makeGeom(h, items.length), [h, items.length]);
  const guide = useMemo(() => arcPath(geom), [geom]);

  const paint = useCallback((pos: number) => {
    itemRefs.current.forEach((el, i) => {
      if (!el) return;
      const d = i - pos;
      const l = layoutFor(d, geom);
      el.style.transform = `translate3d(${l.x.toFixed(2)}px, ${l.y.toFixed(2)}px, 0) scale(${l.scale.toFixed(3)})`;
      el.style.opacity = l.opacity.toFixed(3);
      el.style.pointerEvents = l.opacity < 0.12 ? "none" : "auto";
      el.style.zIndex = String(100 - Math.round(Math.abs(d) * 10));
    });
  }, [geom]);

  // First paint before the browser draws, and whenever the item count changes.
  useIsoLayoutEffect(() => {
    paint(posRef.current);
  }, [paint, items.length]);

  // Spring toward the focused index.
  useEffect(() => {
    const target = Math.min(Math.max(focus, 0), Math.max(items.length - 1, 0));
    if (window.matchMedia?.("(prefers-reduced-motion: reduce)").matches) {
      posRef.current = target;
      velRef.current = 0;
      paint(target);
      return;
    }
    const STIFFNESS = 170;
    const DAMPING = 2 * Math.sqrt(STIFFNESS); // critically damped: no overshoot
    let raf = 0;
    let last = performance.now();
    const step = (now: number) => {
      const dt = Math.min((now - last) / 1000, 0.032);
      last = now;
      const diff = target - posRef.current;
      if (Math.abs(diff) < 0.002 && Math.abs(velRef.current) < 0.01) {
        posRef.current = target;
        velRef.current = 0;
        paint(target);
        return;
      }
      velRef.current += (STIFFNESS * diff - DAMPING * velRef.current) * dt;
      posRef.current += velRef.current * dt;
      paint(posRef.current);
      raf = requestAnimationFrame(step);
    };
    raf = requestAnimationFrame(step);
    return () => cancelAnimationFrame(raf);
  }, [focus, items.length, paint]);

  const onWheel = (e: WheelEvent<HTMLElement>) => {
    const now = performance.now();
    wheelAcc.current += e.deltaY;
    if (Math.abs(wheelAcc.current) < 40 || now - lastWheelStep.current < 110) return;
    const dir = Math.sign(wheelAcc.current);
    wheelAcc.current = 0;
    lastWheelStep.current = now;
    setFocus((f) => Math.min(Math.max(f + dir, 0), items.length - 1));
  };

  const onKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
    const links = itemRefs.current;
    const current = links.findIndex((el) => el === document.activeElement);
    if (current < 0) return;
    const next = Math.min(Math.max(current + (e.key === "ArrowDown" ? 1 : -1), 0), links.length - 1);
    e.preventDefault();
    links[next]?.focus();
  };

  const fade = "linear-gradient(to bottom, transparent, black 16%, black 84%, transparent)";

  return (
    <div ref={wrapRef} className="relative min-h-[22rem] flex-1">
      <nav
        aria-label={ariaLabel}
        onWheel={onWheel}
        onKeyDown={onKeyDown}
        className="absolute inset-0 select-none overflow-hidden"
        style={{ maskImage: fade, WebkitMaskImage: fade }}
      >
        {/* Arc guide on the left edge, with a purple notch at its apex. */}
        <svg
          aria-hidden
          className="pointer-events-none absolute left-0 top-0 text-sidebar-foreground/15"
          width={X_CENTER}
          height={geom.h}
        >
          <path d={guide} fill="none" stroke="currentColor" strokeWidth={1} />
          <rect
            x={X_CENTER - GAP - 1}
            y={geom.h / 2 - 12}
            width={2}
            height={24}
            rx={1}
            style={{ fill: "var(--bubble-purple)" }}
          />
        </svg>

        {items.map((item, i) => (
          <AppLink
            key={item.key}
            ref={(el) => {
              itemRefs.current[i] = el;
            }}
            href={item.href}
            newTabTitle={item.label}
            draggable={false}
            aria-current={item.active ? "page" : undefined}
            onFocus={() => setFocus(i)}
            onClick={() => setFocus(i)}
            className="group/curve absolute left-0 top-0 flex origin-left items-center gap-3 rounded-full will-change-transform outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring"
            style={{ opacity: 0, height: BUBBLE }}
          >
            <span
              className={cn(
                "flex shrink-0 items-center justify-center rounded-full transition-[background-color,border-color,box-shadow,color] duration-300",
                item.active
                  ? "border border-transparent bg-[color-mix(in_oklab,var(--bubble-purple)_30%,transparent)] text-[color-mix(in_oklab,var(--bubble-purple)_75%,var(--sidebar-foreground))]"
                  : "border border-sidebar-foreground/10 bg-transparent text-sidebar-foreground/60 group-hover/curve:border-[color-mix(in_oklab,var(--bubble-purple)_50%,transparent)] group-hover/curve:text-sidebar-foreground",
              )}
              style={{ width: BUBBLE, height: BUBBLE }}
            >
              <item.Icon className="size-5" />
            </span>
            <span
              className={cn(
                "max-w-[9rem] truncate text-[0.9375rem] transition-colors duration-300",
                item.active
                  ? "font-medium text-sidebar-foreground"
                  : "text-sidebar-foreground/55 group-hover/curve:text-sidebar-foreground",
              )}
            >
              {item.label}
            </span>
            {item.badge}
          </AppLink>
        ))}
      </nav>
    </div>
  );
}
