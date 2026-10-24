import type { RfbClient } from "../types";

/**
 * noVNC 1.7.0 only exposes "contain" scaling through its public API. To make
 * the remote framebuffer cover a phone screen we drive its viewport directly:
 * the framebuffer is cropped to the visible viewport and the crop is scaled to
 * the screen size. Drag panning keeps working through noVNC's dragViewport.
 *
 * The dependency is pinned to 1.7.0. Feature detection below makes an upgrade
 * fall back to regular fit scaling instead of breaking the app.
 */

interface ViewportLocation {
  x: number;
  y: number;
  w: number;
  h: number;
}

interface NovncDisplay {
  readonly width: number;
  readonly height: number;
  scale: number;
  clipViewport: boolean;
  viewportChangeSize(width: number, height: number): void;
  viewportChangePos(deltaX: number, deltaY: number): void;
  _viewportLoc?: ViewportLocation;
}

interface NovncRfbInternals {
  _display?: NovncDisplay;
  _screen?: HTMLElement;
  _updateClip?: () => void;
  _updateScale?: () => void;
  _setClippingViewport?: (on: boolean) => void;
}

export interface FramebufferSize {
  width: number;
  height: number;
}

export interface FillViewport {
  readonly supported: boolean;
  setActive(active: boolean): void;
  refresh(): void;
}

const unsupported: FillViewport = {
  supported: false,
  setActive() {},
  refresh() {},
};

export function remoteFramebufferSize(rfb: RfbClient | null): FramebufferSize | null {
  if (!rfb) return null;
  const display = (rfb as unknown as NovncRfbInternals)._display;
  if (!display || !display.width || !display.height) return null;
  return { width: display.width, height: display.height };
}

export function installFillViewport(rfb: RfbClient): FillViewport {
  const internals = rfb as unknown as NovncRfbInternals;
  const display = internals._display;
  if (
    !display ||
    !internals._screen ||
    typeof internals._updateClip !== "function" ||
    typeof internals._updateScale !== "function" ||
    typeof display.viewportChangeSize !== "function" ||
    typeof display.viewportChangePos !== "function"
  ) {
    return unsupported;
  }

  const target: NovncDisplay = display;
  let active = false;
  let scale = 1;

  const originalClip = internals._updateClip.bind(rfb);
  const originalScale = internals._updateScale.bind(rfb);
  const originalChangePos = target.viewportChangePos.bind(target);

  target.viewportChangePos = (deltaX, deltaY) => {
    // noVNC hands us drag deltas in CSS pixels. The viewport is expressed in
    // framebuffer pixels, so undo the current scale to keep the image under
    // the finger.
    const factor = active && scale > 0 ? scale : 1;
    originalChangePos(deltaX / factor, deltaY / factor);
  };

  function apply() {
    if (!active) return;

    const screen = internals._screen;
    if (!screen) return;

    const rect = screen.getBoundingClientRect();
    const width = rect.width;
    const height = rect.height;
    const fbWidth = target.width;
    const fbHeight = target.height;
    if (width <= 0 || height <= 0 || fbWidth <= 0 || fbHeight <= 0) return;

    // Uniform cover scale: fill both axes, cropping the larger overflow.
    const nextScale = Math.max(width / fbWidth, height / fbHeight);
    if (!Number.isFinite(nextScale) || nextScale <= 0) return;
    scale = nextScale;

    if (!target.clipViewport) target.clipViewport = true;

    const viewWidth = Math.max(1, Math.min(fbWidth, Math.floor(width / nextScale)));
    const viewHeight = Math.max(1, Math.min(fbHeight, Math.floor(height / nextScale)));
    target.viewportChangeSize(viewWidth, viewHeight);
    target.scale = nextScale;

    const viewport = target._viewportLoc;
    if (viewport) {
      const targetX = Math.floor((fbWidth - viewWidth) / 2);
      const targetY = Math.floor((fbHeight - viewHeight) / 2);
      if (viewport.x !== targetX || viewport.y !== targetY) {
        // Center through the original method: the public one compensates for
        // finger drags and would divide our centering delta as well.
        originalChangePos(targetX - viewport.x, targetY - viewport.y);
      }
    }

    internals._setClippingViewport?.(viewWidth < fbWidth || viewHeight < fbHeight);
  }

  internals._updateClip = () => {
    if (active) apply();
    else originalClip();
  };

  internals._updateScale = () => {
    if (active) apply();
    else originalScale();
  };

  return {
    supported: true,
    setActive(value: boolean) {
      active = value;
      if (active) apply();
    },
    refresh: apply,
  };
}
