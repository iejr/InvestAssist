import axios from 'axios';

// API client for strategy-server-go. All money fields are in the strategy's
// quote currency; display_* mirrors are in the requested display currency and
// may be null when the display conversion is unavailable at that point.

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
  increment: number;
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

export interface HistoryPoint {
  date: string;
  step_count: number;
  shares_held: number;
  target: number;
  actual: number | null;
  recommended_contribution: number | null;
  display_target: number | null;
  display_actual: number | null;
  display_recommended_contribution: number | null;
  missing: boolean;
}

export interface History {
  strategy_id: string;
  base: string;
  quote: string;
  display: string;
  points: HistoryPoint[];
}

export interface Summary {
  strategy_id: string;
  base: string;
  quote: string;
  display: string;
  shares_held: number;
  target: number;
  actual: number | null;
  current_value: number | null;
  total_invested: number;
  gain_loss: number | null;
  next_recommended_contribution: number | null;
  display_current_value: number | null;
  display_total_invested: number | null;
  display_gain_loss: number | null;
  display_next_recommended_contribution: number | null;
  price_stale: boolean;
}

export interface Transaction {
  id: string;
  strategy_id: string;
  spent_symbol: string;
  spent_amount: number;
  gained_symbol: string;
  gained_amount: number;
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
