import { BrowserRouter, Routes, Route } from 'react-router-dom';
import Home from './pages/Home';
import StrategyDetail from './pages/StrategyDetail';

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/strategy/:id" element={<StrategyDetail />} />
      </Routes>
    </BrowserRouter>
  );
}

export default App;
