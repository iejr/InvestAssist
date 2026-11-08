import axios from 'axios';

// API client for strategy-server-go. All money fields are in the strategy's
// quote currency, serialized as decimal strings (not JS numbers) so they
// round-trip exactly — parse with Number()/parseFloat only for display
// formatting, never for further arithmetic (money math stays server-side).
// display_* mirrors are in the requested display currency and the key is
// absent entirely when that conversion is unavailable at that point.

export type StrategyType = 'VA' | 'DCA';
export type Interval = 'weekly' | 'monthly';

export interface Strategy {
  id: string;
  name: string;
  type: StrategyType;
  base: string;
  quote: string;
  start_date: string;
  interval: Interval;
  increment: string;
}

export interface CreateStrategyReq {
  name: string;
  type: StrategyType;
  base: string;
  quote: string;
  start_date: string;
  interval: Interval;
  increment: number;
}

// SeriesMeta describes one named curve in HistoryPoint.values, so the chart
// can render whatever a strategy exposes without hardcoding which keys
// exist for which strategy type.
export interface SeriesMeta {
  key: string;
  label: string;
}

export interface HistoryPoint {
  date: string;
  step_count: number;
  shares_held: string;
  // Keyed by SeriesMeta.key (see History.series). A key's absence means
  // that value is unknown at this point (e.g. price missing).
  values: Record<string, string>;
  display_values?: Record<string, string>;
  missing: boolean;
}

export interface History {
  strategy_id: string;
  base: string;
  quote: string;
  display: string;
  series: SeriesMeta[];
  points: HistoryPoint[];
}

export interface Summary {
  strategy_id: string;
  base: string;
  quote: string;
  display: string;
  shares_held: string;
  // Known keys: total_invested (always), current_value/gain_loss/
  // next_recommended_contribution (absent when price is stale), plus any
  // strategy-specific extras (e.g. VA's "target").
  values: Record<string, string>;
  display_values?: Record<string, string>;
  price_stale: boolean;
}

export interface Transaction {
  id: string;
  strategy_id: string;
  spent_symbol: string;
  spent_amount: string;
  gained_symbol: string;
  gained_amount: string;
  memo: string;
  timestamp: string;
}

export interface CreateTransactionReq {
  strategy_id: string;
  spent_symbol: string;
  spent_amount: number;
  gained_symbol: string;
  gained_amount: number;
  memo: string;
  timestamp: string;
}

export interface DisplayCurrencies {
  options: string[];
  default: string;
}

export const api = {
  listBases: () => axios.get<string[]>('/bases').then((r) => r.data),
  listQuotes: (base: string) =>
    axios.get<string[]>('/quotes', { params: { base } }).then((r) => r.data),

  listStrategies: () => axios.get<Strategy[]>('/strategies').then((r) => r.data),
  getStrategy: (id: string) => axios.get<Strategy>(`/strategies/${id}`).then((r) => r.data),
  createStrategy: (body: CreateStrategyReq) =>
    axios.post<Strategy>('/strategies', body).then((r) => r.data),

  getHistory: (id: string, display?: string, range?: string) =>
    axios
      .get<History>(`/strategies/${id}/history`, { params: { display, range } })
      .then((r) => r.data),
  getSummary: (id: string, display?: string) =>
    axios.get<Summary>(`/strategies/${id}/summary`, { params: { display } }).then((r) => r.data),
  getDisplayCurrencies: (id: string) =>
    axios.get<DisplayCurrencies>(`/strategies/${id}/display-currencies`).then((r) => r.data),

  listTransactions: (strategyId: string) =>
    axios
      .get<Transaction[]>('/transactions', { params: { strategy_id: strategyId } })
      .then((r) => r.data),
  createTransaction: (body: CreateTransactionReq) =>
    axios.post<Transaction>('/transactions', body).then((r) => r.data),
};
