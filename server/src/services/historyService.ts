import fs from 'fs/promises';
import path from 'path';
import { parseCsvFile } from '../utils/file';
import { getDateSequence } from '../utils/date';

export async function getHistory(strategyId: string) {
  const strategyPath = path.join(__dirname, '../../data/strategies.json');
  const strategyRaw = await fs.readFile(strategyPath, 'utf-8');
  const strategies = JSON.parse(strategyRaw);
  const strategy = strategies.find((s: any) => s.strategy_id === strategyId);
  if (!strategy) throw new Error('Strategy not found');

  const assetPath = path.join(__dirname, `../../data/assets_${strategyId}.csv`);
  const pricePath = path.join(__dirname, `../../data/prices_${strategy.security}.csv`);
  const assets = await parseCsvFile(assetPath);
  const prices = await parseCsvFile(pricePath);

  const dateSeq = getDateSequence(strategy.start_date, strategy.interval);
  let totalShares = 0;

  const result = dateSeq.map(date => {
    const assetEntry = assets.find(a => a[0] === date);
    if (assetEntry) totalShares = parseFloat(assetEntry[1]);

    const priceEntry = prices.find(p => p[0] === date);
    const price = priceEntry ? parseFloat(priceEntry[1]) : 0;

    const actual_value = totalShares * price;
    const intervalCount = dateSeq.indexOf(date);
    const expected_value = intervalCount * strategy.increment;

    return { date, actual_value, expected_value };
  });

  return result;
}