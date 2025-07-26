import path from 'path';
import { appendCsvRows } from '../utils/file';

export async function appendPrices(security: string, prices: Array<{ date: string; price: number }>) {
  const filePath = path.join(__dirname, `../../data/prices_${security}.csv`);
  const rows = prices.map(p => [p.date, p.price]);
  await appendCsvRows(filePath, rows);
}
