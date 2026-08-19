const retryDelays = [250, 500, 1_000, 2_000, 5_000] as const;

export function retryDelay(attempt: number): number {
  const index = Math.max(0, Math.min(attempt, retryDelays.length - 1));
  return retryDelays[index];
}
