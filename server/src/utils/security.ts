export const ALLOWED_SECURITIES = ['SPY500', 'QQQ', 'BTC'];

export function isValidSecurity(symbol: string): boolean {
  return ALLOWED_SECURITIES.includes(symbol);
}
