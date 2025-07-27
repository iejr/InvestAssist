import { useParams } from 'react-router-dom';

export default function StrategyDetail() {
  const { id } = useParams();

  return (
    <div className="p-4">
      <h1 className="text-2xl font-bold">Strategy Detail: {id}</h1>
      <p>More info coming soon...</p>
    </div>
  );
}
