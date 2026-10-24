export const MODIFIER_KEYS = {
  control: { keysym: 0xffe3, code: "ControlLeft", label: "Ctrl" },
  alt: { keysym: 0xffe9, code: "AltLeft", label: "Alt" },
  super: { keysym: 0xffeb, code: "MetaLeft", label: "Super" },
  shift: { keysym: 0xffe1, code: "ShiftLeft", label: "Shift" },
} as const;

export const SPECIAL_KEYS = {
  escape: { keysym: 0xff1b, code: "Escape", label: "Esc" },
  tab: { keysym: 0xff09, code: "Tab", label: "Tab" },
  enter: { keysym: 0xff0d, code: "Enter", label: "Enter" },
  backspace: { keysym: 0xff08, code: "Backspace", label: "⌫" },
  left: { keysym: 0xff51, code: "ArrowLeft", label: "←" },
  down: { keysym: 0xff54, code: "ArrowDown", label: "↓" },
  up: { keysym: 0xff52, code: "ArrowUp", label: "↑" },
  right: { keysym: 0xff53, code: "ArrowRight", label: "→" },
} as const;

export type ModifierName = keyof typeof MODIFIER_KEYS;
export type SpecialName = keyof typeof SPECIAL_KEYS;

export function keysymForCharacter(character: string): number {
  const codepoint = character.codePointAt(0);
  if (!codepoint) return 0;
  if (codepoint >= 0x20 && codepoint <= 0xff) return codepoint;
  return 0x01000000 | codepoint;
}
