import { useParams } from 'react-router-dom';
import { useCallback, useEffect, useState } from 'react';
import { Line } from 'react-chartjs-2';
import {
  Chart as ChartJS,
  LineElement,
  CategoryScale,
  LinearScale,
  PointElement,
  Tooltip,
  Legend,
} from 'chart.js';
import {
  api,
  type Strategy,
  type History,
  type Summary,
  type Transaction,
  type TxType,
} from '../api/strategy';

ChartJS.register(LineElement, CategoryScale, LinearScale, PointElement, Tooltip, Legend);

export default function StrategyDetail() {
  const { id } = useParams();
  const [strategy, setStrategy] = useState<Strategy | null>(null);
  const [history, setHistory] = useState<History | null>(null);
  const [summary, setSummary] = useState<Summary | null>(null);
  const [transactions, setTransactions] = useState<Transaction[]>([]);

  const [displayOptions, setDisplayOptions] = useState<string[]>([]);
  const [display, setDisplay] = useState<string>('');

  const [tx, setTx] = useState({
    type: 'BUY' as TxType,
    shares: '',
    price: '',
    fee_currency: '',
    fee_amount: '',
    timestamp: '',
  });

  const refresh = useCallback(() => {
    if (!id) return;
    api.getStrategy(id).then(setStrategy).catch((e) => console.error('strategy', e));
    api.getHistory(id, display || undefined).then(setHistory).catch((e) => console.error('history', e));
    api.getSummary(id, display || undefined).then(setSummary).catch((e) => console.error('summary', e));
    api.listTransactions(id).then(setTransactions).catch((e) => console.error('transactions', e));
  }, [id, display]);

  useEffect(() => {
    if (!id) return;
    api
      .getDisplayCurrencies(id)
      .then((d) => {
        setDisplayOptions(d.options);
        setDisplay((prev) => prev || d.default);
      })
      .catch((e) => console.error('display currencies', e));
  }, [id]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const handleTxChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const { name, value } = e.target;
    setTx((prev) => ({ ...prev, [name]: value }));
  };

  const submitTx = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!id) return;
    await api.createTransaction({
      strategy_id: id,
      type: tx.type,
      shares: parseFloat(tx.shares),
      price: parseFloat(tx.price),
      fee_currency: tx.fee_currency.trim(),
      fee_amount: tx.fee_amount ? parseFloat(tx.fee_amount) : 0,
      timestamp: tx.timestamp,
    });
    setTx({ type: 'BUY', shares: '', price: '', fee_currency: '', fee_amount: '', timestamp: '' });
    refresh();
  };

  // Chart uses display-converted values, falling back to quote values.
  const points = history?.points ?? [];
  const chartData = {
    labels: points.map((p) => p.date.slice(0, 10)),
    datasets: [
      {
        label: `Target (${display})`,
        data: points.map((p) => p.display_target ?? p.target),
        borderColor: 'rgb(34, 197, 94)',
        backgroundColor: 'rgba(34, 197, 94, 0.3)',
        tension: 0.3,
      },
      {
        label: `Actual (${display})`,
        data: points.map((p) => p.display_actual ?? p.actual),
        borderColor: 'rgb(59, 130, 246)',
        backgroundColor: 'rgba(59, 130, 246, 0.3)',
        tension: 0.3,
      },
    ],
  };

  const money = (v: number | null | undefined) =>
    v == null ? '—' : `${v.toFixed(2)} ${display}`;

  return (
    <div className="p-4 max-w-4xl mx-auto space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">
          {strategy ? `${strategy.name} · ${strategy.base}/${strategy.quote}` : `Strategy #${id}`}
        </h1>
        <label className="text-sm">
          <span className="text-gray-600 mr-2">Display</span>
          <select
            value={display}
            onChange={(e) => setDisplay(e.target.value)}
            className="border px-2 py-1"
          >
            {displayOptions.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        </label>
      </div>

      {summary && (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3 text-sm">
          <Stat label="Current Value" value={money(summary.display_current_value ?? summary.current_value)} />
          <Stat label="Total Invested" value={money(summary.display_total_invested ?? summary.total_invested)} />
          <Stat label="Gain / Loss" value={money(summary.display_gain_loss ?? summary.gain_loss)} />
          <Stat
            label="Next Recommended"
            value={money(summary.display_next_recommended_contribution ?? summary.next_recommended_contribution)}
          />
          {summary.price_stale && (
            <p className="col-span-full text-amber-600">
              Live price unavailable for {summary.base}/{summary.quote} — value may be stale.
            </p>
          )}
        </div>
      )}

      <div className="bg-white p-4 shadow rounded">
        <Line data={chartData} />
        {points.some((p) => p.missing) && (
          <p className="text-xs text-amber-600 mt-2">
            Some points have no priced observation within the staleness window and are omitted from the actual line.
          </p>
        )}
      </div>

      <form onSubmit={submitTx} className="bg-white p-4 shadow rounded space-y-3">
        <h2 className="text-lg font-semibold">Record Transaction</h2>
        <div className="grid grid-cols-2 md:grid-cols-3 gap-3">
          <label className="block">
            <span className="text-sm text-gray-600">Type</span>
            <select name="type" value={tx.type} onChange={handleTxChange} className="w-full border px-2 py-1">
              <option value="BUY">Buy</option>
              <option value="SELL">Sell</option>
            </select>
          </label>
          <label className="block">
            <span className="text-sm text-gray-600">Date</span>
            <input name="timestamp" type="date" value={tx.timestamp} onChange={handleTxChange} className="w-full border px-2 py-1" required />
          </label>
          <label className="block">
            <span className="text-sm text-gray-600">Shares</span>
            <input name="shares" type="number" step="any" value={tx.shares} onChange={handleTxChange} className="w-full border px-2 py-1" required />
          </label>
          <label className="block">
            <span className="text-sm text-gray-600">Price ({strategy?.quote ?? 'quote'})</span>
            <input name="price" type="number" step="any" value={tx.price} onChange={handleTxChange} className="w-full border px-2 py-1" required />
          </label>
          <label className="block">
            <span className="text-sm text-gray-600">Fee currency</span>
            <input name="fee_currency" placeholder="e.g. USDT" value={tx.fee_currency} onChange={handleTxChange} className="w-full border px-2 py-1" />
          </label>
          <label className="block">
            <span className="text-sm text-gray-600">Fee amount</span>
            <input name="fee_amount" type="number" step="any" value={tx.fee_amount} onChange={handleTxChange} className="w-full border px-2 py-1" />
          </label>
        </div>
        <button type="submit" className="bg-blue-600 text-white px-4 py-2 rounded hover:bg-blue-700">
          Add Transaction
        </button>
      </form>

      <div className="bg-white p-4 shadow rounded">
        <h2 className="text-lg font-semibold mb-2">Transactions</h2>
        {transactions.length === 0 ? (
          <p className="text-sm text-gray-500">No transactions recorded.</p>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-gray-500">
                <th className="py-1">Date</th>
                <th>Type</th>
                <th>Shares</th>
                <th>Price</th>
                <th>Fee</th>
              </tr>
            </thead>
            <tbody>
              {transactions.map((t) => (
                <tr key={t.id} className="border-t">
                  <td className="py-1">{t.timestamp.slice(0, 10)}</td>
                  <td>{t.type}</td>
                  <td>{t.shares}</td>
                  <td>{t.price}</td>
                  <td>{t.fee_amount ? `${t.fee_amount} ${t.fee_currency}` : '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-gray-50 border rounded p-3">
      <p className="text-gray-500">{label}</p>
      <p className="text-base font-semibold">{value}</p>
    </div>
  );
}
