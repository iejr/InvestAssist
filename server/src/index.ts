import express from 'express';
import strategyRoutes from './routes/strategy';
import priceRoutes from './routes/price';
import historyRoutes from './routes/history';
import securityRoutes from './routes/security';

const app = express();
const PORT = 3000;

app.use(express.json());

app.use('/strategy', strategyRoutes);
app.use('/price', priceRoutes);
app.use('/history', historyRoutes);
app.use('/securities', securityRoutes);

app.listen(PORT, () => {
  console.log(`VA Simulator Backend running on port ${PORT}`);
});

// Folder structure (src/):
// - routes/
//     - strategy.ts
//     - price.ts
//     - history.ts
// - services/
//     - strategyService.ts
//     - assetService.ts
//     - priceService.ts
//     - historyService.ts
// - utils/
//     - file.ts
//     - date.ts

// Common formats:
// - Date: YYYY-MM-DD (ISO 8601 string)
// - Currency: USD
// - Prices and values in decimal (e.g., 427.83)
// - Shares in decimal (e.g., 1.25)