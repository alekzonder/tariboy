// Usage cost in US dollars, at the four-decimal precision the proxy reports.
export function fmtCost(usd: number): string {
  return `$${usd.toFixed(4)}`;
}
