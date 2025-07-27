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

  useEffect(() => {
    axios.get('/strategy') // backend proxy through Vite
      .then(res => setStrategies(res.data))
      .catch(err => console.error('Failed to load strategies:', err));
  }, []);

  return (
    <div className="p-4 max-w-3xl mx-auto">
      <h1 className="text-2xl font-bold mb-4">Value Average Strategies</h1>
      <ul className="space-y-4">
        {strategies.map(s => (
          <li key={s.strategy_id} className="border p-4 rounded shadow">
            <h2 className="text-xl font-semibold">{s.name}</h2>
            <p>Start: {s.start_date}, Interval: {s.interval}, Increment: ${s.increment}</p>
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
