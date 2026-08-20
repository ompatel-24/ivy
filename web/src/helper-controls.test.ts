import { HelperControls, helperKeyBytes, helperKeyNames, type HelperKeyName } from "./helper-controls";

const expectedBytes: Record<HelperKeyName, number[]> = {
  "ctrl-c": [0x03],
  "ctrl-d": [0x04],
  escape: [0x1b],
  tab: [0x09],
  up: [0x1b, 0x5b, 0x41],
  down: [0x1b, 0x5b, 0x42],
  left: [0x1b, 0x5b, 0x44],
  right: [0x1b, 0x5b, 0x43],
};

function controlsElement(): HTMLElement {
  const root = document.createElement("nav");
  for (const name of helperKeyNames) {
    const button = document.createElement("button");
    button.dataset.helperKey = name;
    button.textContent = name;
    root.append(button);
  }
  document.body.append(root);
  return root;
}

describe("mobile helper controls", () => {
  afterEach(() => {
    document.body.replaceChildren();
  });

  it.each(helperKeyNames)("maps %s to its exact terminal bytes", (name) => {
    expect(Array.from(helperKeyBytes(name))).toEqual(expectedBytes[name]);
  });

  it("stays disabled until live and sends exactly once per tap", () => {
    const root = controlsElement();
    const sendInput = vi.fn().mockReturnValue(true);
    const focusTerminal = vi.fn();
    const controls = new HelperControls(root, sendInput, focusTerminal);
    const ctrlC = root.querySelector<HTMLButtonElement>('[data-helper-key="ctrl-c"]')!;

    ctrlC.click();
    expect(sendInput).not.toHaveBeenCalled();
    expect(root.dataset.enabled).toBe("false");

    controls.setEnabled(true);
    ctrlC.click();
    expect(sendInput).toHaveBeenCalledOnce();
    expect(sendInput).toHaveBeenCalledWith(Uint8Array.of(0x03));
    expect(focusTerminal).toHaveBeenCalledOnce();

    controls.setEnabled(false);
    ctrlC.click();
    expect(sendInput).toHaveBeenCalledOnce();
    controls.dispose();
  });

  it("prevents pointer focus movement while enabled", () => {
    const root = controlsElement();
    const controls = new HelperControls(root, () => true, vi.fn());
    const escape = root.querySelector<HTMLButtonElement>('[data-helper-key="escape"]')!;

    const disabledEvent = new PointerEvent("pointerdown", { bubbles: true, cancelable: true });
    escape.dispatchEvent(disabledEvent);
    expect(disabledEvent.defaultPrevented).toBe(false);

    controls.setEnabled(true);
    const enabledEvent = new PointerEvent("pointerdown", { bubbles: true, cancelable: true });
    escape.dispatchEvent(enabledEvent);
    expect(enabledEvent.defaultPrevented).toBe(true);
    controls.dispose();
  });

  it("does not focus the terminal when the transport rejects input", () => {
    const root = controlsElement();
    const focusTerminal = vi.fn();
    const controls = new HelperControls(root, () => false, focusTerminal);
    controls.setEnabled(true);

    root.querySelector<HTMLButtonElement>('[data-helper-key="tab"]')!.click();
    expect(focusTerminal).not.toHaveBeenCalled();
    controls.dispose();
  });
});
