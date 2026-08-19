export class SizeTracker {
  private last = "";

  shouldSend(cols: number, rows: number, force = false): boolean {
    const current = `${cols}x${rows}`;
    if (!force && current === this.last) {
      return false;
    }
    this.last = current;
    return true;
  }
}
