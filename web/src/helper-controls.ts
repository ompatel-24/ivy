export const helperKeyNames = ["ctrl-c", "ctrl-d", "escape", "tab", "up", "down", "left", "right"] as const;

export type HelperKeyName = (typeof helperKeyNames)[number];

const helperKeyData: Record<HelperKeyName, readonly number[]> = {
  "ctrl-c": [0x03],
  "ctrl-d": [0x04],
  escape: [0x1b],
  tab: [0x09],
  up: [0x1b, 0x5b, 0x41],
  down: [0x1b, 0x5b, 0x42],
  left: [0x1b, 0x5b, 0x44],
  right: [0x1b, 0x5b, 0x43],
};

export function helperKeyBytes(name: HelperKeyName): Uint8Array<ArrayBuffer> {
  return Uint8Array.from(helperKeyData[name]);
}

function isHelperKeyName(value: string | undefined): value is HelperKeyName {
  return value !== undefined && helperKeyNames.includes(value as HelperKeyName);
}

export class HelperControls {
  private readonly buttons: HTMLButtonElement[];

  constructor(
    private readonly root: HTMLElement,
    private readonly sendInput: (data: Uint8Array<ArrayBuffer>) => boolean,
    private readonly focusTerminal: () => void,
  ) {
    this.buttons = Array.from(root.querySelectorAll<HTMLButtonElement>("button[data-helper-key]"));
    root.addEventListener("pointerdown", this.handlePointerDown);
    root.addEventListener("click", this.handleClick);
    this.setEnabled(false);
  }

  setEnabled(enabled: boolean): void {
    for (const button of this.buttons) {
      button.disabled = !enabled;
    }
    this.root.dataset.enabled = String(enabled);
  }

  dispose(): void {
    this.root.removeEventListener("pointerdown", this.handlePointerDown);
    this.root.removeEventListener("click", this.handleClick);
  }

  private readonly handlePointerDown = (event: PointerEvent): void => {
    const button = this.buttonForEvent(event);
    if (button && !button.disabled) {
      event.preventDefault();
    }
  };

  private readonly handleClick = (event: MouseEvent): void => {
    const button = this.buttonForEvent(event);
    if (!button || button.disabled) {
      return;
    }
    const name = button.dataset.helperKey;
    if (!isHelperKeyName(name)) {
      return;
    }
    if (this.sendInput(helperKeyBytes(name))) {
      this.focusTerminal();
    }
  };

  private buttonForEvent(event: Event): HTMLButtonElement | null {
    if (!(event.target instanceof Element)) {
      return null;
    }
    const button = event.target.closest<HTMLButtonElement>("button[data-helper-key]");
    return button && this.root.contains(button) ? button : null;
  }
}
