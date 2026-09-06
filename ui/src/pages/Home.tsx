import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api, type Strategy, type StrategyType, type Interval } from '../api/strategy';

export default function Home() {
  const [strategies, setStrategies] = useState<Strategy[]>([]);
  const [bases, setBases] = useState<string[]>([]);
  const [quotes, setQuotes] = useState<string[]>([]);

  const [form, setForm] = useState({
    name: '',
    type: 'VA' as StrategyType,
    base: '',
    quote: '',
    start_date: '',
    interval: 'monthly' as Interval,
    increment: '',
  });

  const refreshStrategies = () => {
    api.listStrategies().then(setStrategies).catch((e) => console.error('load strategies', e));
  };

  useEffect(() => {
    refreshStrategies();
    api.listBases().then(setBases).catch((e) => console.error('load bases', e));
  }, []);

  // Quotes depend on the chosen base — derived from what the feed can price.
  useEffect(() => {
    if (!form.base) {
      setQuotes([]);
      return;
    }
    api
      .listQuotes(form.base)
      .then((qs) => {
        setQuotes(qs);
        setForm((prev) => ({ ...prev, quote: qs.includes(prev.quote) ? prev.quote : qs[0] ?? '' }));
      })
      .catch((e) => console.error('load quotes', e));
  }, [form.base]);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const { name, value } = e.target;
    setForm((prev) => ({ ...prev, [name]: value }));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!form.base || !form.quote) return;
    await api.createStrategy({
      name: form.name,
      type: form.type,
      base: form.base,
      quote: form.quote,
      start_date: form.start_date,
      interval: form.interval,
      increment: parseFloat(form.increment),
    });
    setForm({ name: '', type: 'VA', base: '', quote: '', start_date: '', interval: 'monthly', increment: '' });
    refreshStrategies();
  };

  return (
    <div className="p-4 max-w-3xl mx-auto space-y-6">
      <h1 className="text-2xl font-bold">Value-Average / DCA Strategies</h1>

      <form onSubmit={handleSubmit} className="space-y-4 border p-4 rounded shadow bg-white">
        <h2 className="text-xl font-semibold">Create New Strategy</h2>

        <input
          name="name"
          placeholder="Strategy Name"
          value={form.name}
          onChange={handleChange}
          className="w-full border px-2 py-1"
          required
        />

        <div className="grid grid-cols-2 gap-4">
          <label className="block">
            <span className="text-sm text-gray-600">Type</span>
            <select name="type" value={form.type} onChange={handleChange} className="w-full border px-2 py-1">
              <option value="VA">Value Average (VA)</option>
              <option value="DCA">Dollar-Cost Averaging (DCA)</option>
            </select>
          </label>

          <label className="block">
            <span className="text-sm text-gray-600">Interval</span>
            <select name="interval" value={form.interval} onChange={handleChange} className="w-full border px-2 py-1">
              <option value="weekly">Weekly</option>
              <option value="monthly">Monthly</option>
            </select>
          </label>

          <label className="block">
            <span className="text-sm text-gray-600">Base (asset)</span>
            <select name="base" value={form.base} onChange={handleChange} className="w-full border px-2 py-1" required>
              <option value="" disabled>
                Select base…
              </option>
              {bases.map((b) => (
                <option key={b} value={b}>
                  {b}
                </option>
              ))}
            </select>
          </label>

          <label className="block">
            <span className="text-sm text-gray-600">Quote (accounting ccy)</span>
            <select
              name="quote"
              value={form.quote}
              onChange={handleChange}
              className="w-full border px-2 py-1"
              required
              disabled={!form.base}
            >
              {quotes.length === 0 && <option value="">No quotes available</option>}
              {quotes.map((q) => (
                <option key={q} value={q}>
                  {q}
                </option>
              ))}
            </select>
          </label>
        </div>

        <div className="grid grid-cols-2 gap-4">
          <label className="block">
            <span className="text-sm text-gray-600">Start date</span>
            <input
              name="start_date"
              type="date"
              value={form.start_date}
              onChange={handleChange}
              className="w-full border px-2 py-1"
              required
            />
          </label>

          <label className="block">
            <span className="text-sm text-gray-600">Increment (per step, in quote)</span>
            <input
              name="increment"
              type="number"
              step="0.01"
              placeholder="e.g. 100"
              value={form.increment}
              onChange={handleChange}
              className="w-full border px-2 py-1"
              required
            />
          </label>
        </div>

        <button type="submit" className="bg-blue-600 text-white px-4 py-2 rounded hover:bg-blue-700">
          Create Strategy
        </button>
      </form>

      <ul className="space-y-4">
        {strategies.map((s) => (
          <li key={s.id} className="border p-4 rounded shadow bg-white">
            <h2 className="text-lg font-semibold">
              {s.name} <span className="text-sm text-gray-500">({s.type})</span>
            </h2>
            <p className="text-sm text-gray-700">
              {s.base}/{s.quote} · {s.interval} · increment {s.increment} {s.quote} · from {s.start_date?.slice(0, 10)}
            </p>
            <Link to={`/strategy/${s.id}`} className="text-blue-500 hover:underline">
              View Details →
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
