import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import axios from 'axios';

interface Strategy {
  strategy_id: string;
  name: string;
  start_date: string;
  interval: string;
  increment: number;
  security: string;
}

export default function Home() {
  const [strategies, setStrategies] = useState<Strategy[]>([]);
  const [form, setForm] = useState({
    name: '',
    start_date: '',
    interval: 'weekly',
    increment: '',
  });

  const fetchStrategies = () => {
    axios.get('/strategy')
      .then(res => setStrategies(res.data))
      .catch(err => console.error('Failed to load strategies:', err));
  };

  useEffect(() => {
    fetchStrategies();
  }, []);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const { name, value } = e.target;
    setForm(prev => ({ ...prev, [name]: value }));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const newStrategy = {
      ...form,
      increment: parseFloat(form.increment),
      security: 'SPY500',
    };
    await axios.post('/strategy', newStrategy);
    setForm({ name: '', start_date: '', interval: 'weekly', increment: '' });
    fetchStrategies();
  };

  return (
    <div className="p-4 max-w-3xl mx-auto space-y-6">
      <h1 className="text-2xl font-bold">Value Average Strategies</h1>

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

        <input
          name="start_date"
          type="date"
          value={form.start_date}
          onChange={handleChange}
          className="w-full border px-2 py-1"
          required
        />

        <select
          name="interval"
          value={form.interval}
          onChange={handleChange}
          className="w-full border px-2 py-1"
        >
          <option value="weekly">Weekly</option>
          <option value="monthly">Monthly</option>
        </select>

        <input
          name="increment"
          type="number"
          step="0.01"
          placeholder="Increment Amount (e.g., 100)"
          value={form.increment}
          onChange={handleChange}
          className="w-full border px-2 py-1"
          required
        />

        <button
          type="submit"
          className="bg-blue-600 text-white px-4 py-2 rounded hover:bg-blue-700"
        >
          Create Strategy
        </button>
      </form>

      <ul className="space-y-4">
        {strategies.map(s => (
          <li key={s.strategy_id} className="border p-4 rounded shadow bg-white">
            <h2 className="text-lg font-semibold">{s.name}</h2>
            <p>
              Start: {s.start_date} | Interval: {s.interval} | Increment: ${s.increment}
            </p>
            <Link
              to={`/strategy/${s.strategy_id}`}
              className="text-blue-500 hover:underline"
            >
              View Details →
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
