import { useEffect, useState } from "react";

export type Viewport = "phone" | "tablet" | "desktop";

export const BREAKPOINTS = {
  phoneMax: 639,
  tabletMax: 1023,
  /** Content width where tablet may show list+reading side by side. */
  tabletSplitMin: 768,
} as const;

export function viewportFromWidth(width: number): Viewport {
  if (width <= BREAKPOINTS.phoneMax) return "phone";
  if (width <= BREAKPOINTS.tabletMax) return "tablet";
  return "desktop";
}

export function useViewport(): Viewport {
  const [vp, setVp] = useState<Viewport>(() =>
    typeof window === "undefined" ? "desktop" : viewportFromWidth(window.innerWidth),
  );

  useEffect(() => {
    const mqPhone = window.matchMedia(`(max-width: ${BREAKPOINTS.phoneMax}px)`);
    const mqTablet = window.matchMedia(
      `(min-width: ${BREAKPOINTS.phoneMax + 1}px) and (max-width: ${BREAKPOINTS.tabletMax}px)`,
    );
    const update = () =>
      setVp((prev) => {
        const next = viewportFromWidth(window.innerWidth);
        return prev === next ? prev : next;
      });
    update();
    mqPhone.addEventListener("change", update);
    mqTablet.addEventListener("change", update);
    window.addEventListener("resize", update);
    return () => {
      mqPhone.removeEventListener("change", update);
      mqTablet.removeEventListener("change", update);
      window.removeEventListener("resize", update);
    };
  }, []);

  return vp;
}

export function useContentWidth(ref: { current: HTMLElement | null }): number {
  const [w, setW] = useState(() =>
    typeof window === "undefined" ? BREAKPOINTS.tabletMax + 1 : window.innerWidth,
  );
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const measure = () => {
      const next = Math.round(el.clientWidth);
      setW((prev) => (prev === next ? prev : next));
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [ref]);
  return w;
}
