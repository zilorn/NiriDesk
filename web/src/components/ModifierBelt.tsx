import { For } from "solid-js";
import { useApp } from "../context";
import { MODIFIER_KEYS, SPECIAL_KEYS } from "../lib/keys";
import type { ModifierName, SpecialName } from "../lib/keys";

const modifierEntries = Object.entries(MODIFIER_KEYS) as [ModifierName, (typeof MODIFIER_KEYS)[ModifierName]][];
const specialEntries = Object.entries(SPECIAL_KEYS) as [SpecialName, (typeof SPECIAL_KEYS)[SpecialName]][];

export function ModifierBelt() {
  const app = useApp();
  return (
    <div class="modifier-belt">
      <div class="modifier-group" aria-label="修饰键">
        <For each={modifierEntries}>
          {([name, key]) => (
            <button
              class={`key-button modifier${app.isModifierActive(name) ? " active" : ""}`}
              type="button"
              aria-pressed={app.isModifierActive(name)}
              onPointerDown={(event) => event.preventDefault()}
              onClick={() => app.toggleModifier(name)}
            >
              {key.label}
            </button>
          )}
        </For>
      </div>
      <span class="belt-divider" />
      <div class="modifier-group" aria-label="常用按键">
        <For each={specialEntries}>
          {([name, key]) => (
            <button
              class="key-button"
              type="button"
              onPointerDown={(event) => event.preventDefault()}
              onClick={() => app.sendSpecial(name)}
            >
              {key.label}
            </button>
          )}
        </For>
      </div>
      <button
        class="belt-clear"
        type="button"
        title="释放所有修饰键"
        onPointerDown={(event) => event.preventDefault()}
        onClick={() => {
          app.clearModifiers();
          app.toast("已释放所有修饰键");
        }}
      >
        释放
      </button>
    </div>
  );
}
