import { useParams } from 'react-router-dom';
import { useEffect, useState } from 'react';
import axios from 'axios';
import { Line } from 'react-chartjs-2';
import Papa from 'papaparse';
import {
  Chart as ChartJS,
  LineElement,
  CategoryScale,
  LinearScale,
  PointElement,
  Tooltip,
  Legend,
} from 'chart.js';

ChartJS.register(LineElement, CategoryScale, LinearScale, PointElement, Tooltip, Legend);

interface Strategy {
  strategy_id: string;
  name: string;
  start_date: string;
  interval: string;
  increment: number;
  security: string;
}

interface HistoryPoint {
  date: string;
  actual_value: number;
  expected_value: number;
}

export default function StrategyDetail() {
  const { id } = useParams();
  const [data, setData] = useState<HistoryPoint[]>([]);
  const [strategy, setStrategy] = useState<Strategy | null>(null);

  useEffect(() => {
    axios.get(`/history/${id}`)
      .then(res => setData(res.data))
      .catch(err => console.error(err));

    axios.get(`/strategy`)
      .then(res => {
        const match = res.data.find((s: Strategy) => s.strategy_id === id);
        setStrategy(match);
      })
      .catch(err => console.error(err));
  }, [id]);

  const chartData = {
    labels: data.map(d => d.date),
    datasets: [
      {
        label: 'Actual Value',
        data: data.map(d => d.actual_value),
        borderColor: 'rgb(59, 130, 246)',
        backgroundColor: 'rgba(59, 130, 246, 0.3)',
        tension: 0.3,
      },
      {
        label: 'Expected Value',
        data: data.map(d => d.expected_value),
        borderColor: 'rgb(34, 197, 94)',
        backgroundColor: 'rgba(34, 197, 94, 0.3)',
        tension: 0.3,
      },
    ],
  };

  // Upload CSV handlers
  const handleAssetUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file || !id) return;

    Papa.parse(file, {
      header: true,
      skipEmptyLines: true,
      complete: async (results: Papa.ParseResult<any>) => {
        for (const row of results.data) {
          await axios.post(`/strategy/${id}/asset`, {
            date: row.date,
            shares: parseFloat(row.shares),
          });
        }
        // Refresh history
        const updated = await axios.get(`/history/${id}`);
        setData(updated.data);
      },
    });
  };

  const handlePriceUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file || !strategy?.security) return;

    Papa.parse(file, {
      header: true,
      skipEmptyLines: true,
      complete: async (results: Papa.ParseResult<any>) => {
        const priceData = results.data.map(row => ({
          date: row.date,
          price: parseFloat(row.price),
        }));
        await axios.post(`/price/${strategy.security}`, priceData);

        // Refresh history
        const updated = await axios.get(`/history/${id}`);
        setData(updated.data);
      },
    });
  };


  return (
    <div className="p-4 max-w-4xl mx-auto">
      <h1 className="text-2xl font-bold mb-4">Strategy #{id}</h1>

      <div className="bg-white p-4 shadow rounded">
        <Line data={chartData} />
      </div>

      <div className="bg-white p-4 shadow rounded space-y-4">
        <div>
          <label className="font-semibold block mb-1">Upload Asset CSV</label>
          <input type="file" accept=".csv" onChange={handleAssetUpload} />
        </div>

        <div>
          <label className="font-semibold block mb-1">Upload Price CSV</label>
          <input type="file" accept=".csv" onChange={handlePriceUpload} />
        </div>
      </div>
    </div>
  );
}
