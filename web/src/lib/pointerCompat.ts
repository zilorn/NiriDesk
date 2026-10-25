import type { RfbClient } from "../types";

/**
 * noVNC 1.7.0 with dragViewport enabled delays the button press until
 * mouseup and then sends press and release back to back. Applications that
 * poll GetAsyncKeyState or DirectInput never see the pressed state, so a
 * click looks completely ineffective even though pointer position and focus
 * work. Hold the synthesized press for a short interval so the remote side
 * can observe it.
 *
 * The dependency is pinned to 1.7.0. Feature detection makes this a no-op
 * if the internal method disappears after an upgrade.
 */

interface RfbMouseInternals {
  _sendMouse?: (x: number, y: number, buttonMask: number) => void;
}

const MIN_PRESS_MS = 50;

export function installClickReleaseDelay(rfb: RfbClient): () => void {
  const internals = rfb as unknown as RfbMouseInternals;
  const sendMouse = internals._sendMouse;
  if (typeof sendMouse !== "function") return () => {};

  const originalSendMouse = sendMouse.bind(rfb);
  let pressedMask = 0;
  let pressStartedAt = 0;
  let releaseTimer: number | undefined;
  let releasePoint: { x: number; y: number } | null = null;

  function clearPendingRelease() {
    if (releaseTimer !== undefined) {
      window.clearTimeout(releaseTimer);
      releaseTimer = undefined;
    }
    releasePoint = null;
  }

  function sendPendingRelease() {
    if (!releasePoint) return;
    const point = releasePoint;
    clearPendingRelease();
    originalSendMouse(point.x, point.y, 0);
  }

  internals._sendMouse = (x, y, mask) => {
    if (rfb.viewOnly) {
      clearPendingRelease();
      pressedMask = 0;
      originalSendMouse(x, y, mask);
      return;
    }

    const now = performance.now();

    if (mask === 0 && pressedMask !== 0) {
      const elapsed = now - pressStartedAt;
      if (elapsed >= MIN_PRESS_MS) {
        clearPendingRelease();
        pressedMask = 0;
        originalSendMouse(x, y, 0);
        return;
      }

      clearPendingRelease();
      pressedMask = 0;
      releasePoint = { x, y };
      releaseTimer = window.setTimeout(() => {
        releaseTimer = undefined;
        const point = releasePoint;
        releasePoint = null;
        if (point) originalSendMouse(point.x, point.y, 0);
      }, MIN_PRESS_MS - elapsed);
      return;
    }

    if (mask !== 0 && pressedMask === 0) {
      // Keep ordering if a release is still waiting for its minimum hold.
      sendPendingRelease();
      pressedMask = mask;
      pressStartedAt = now;
      originalSendMouse(x, y, mask);
      return;
    }

    if (mask === 0) {
      // A buttonless move must not overtake a pending release.
      sendPendingRelease();
      originalSendMouse(x, y, 0);
      return;
    }

    // Mask change while pressed, or a move with the same buttons held.
    pressedMask = mask;
    originalSendMouse(x, y, mask);
  };

  return () => {
    clearPendingRelease();
    pressedMask = 0;
    internals._sendMouse = sendMouse;
  };
}
