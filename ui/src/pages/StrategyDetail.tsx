import { useParams } from 'react-router-dom';
import { useEffect, useState } from 'react';
import axios from 'axios';
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

ChartJS.register(LineElement, CategoryScale, LinearScale, PointElement, Tooltip, Legend);

interface HistoryPoint {
  date: string;
  actual_value: number;
  expected_value: number;
}

export default function StrategyDetail() {
  const { id } = useParams();
  const [data, setData] = useState<HistoryPoint[]>([]);

  useEffect(() => {
    axios.get(`/history/${id}`)
      .then(res => setData(res.data))
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

  return (
    <div className="p-4 max-w-4xl mx-auto">
      <h1 className="text-2xl font-bold mb-4">Strategy #{id}</h1>
      <div className="bg-white p-4 shadow rounded">
        <Line data={chartData} />
      </div>
    </div>
  );
}
