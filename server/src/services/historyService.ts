import fs from 'fs/promises';
import path from 'path';
import { parseCsvFile } from '../utils/file';
import { getDateSequence } from '../utils/date';
import { computeHoldings } from './transactionService';

export async function getHistory(strategyId: string) {
  const strategyPath = path.join(__dirname, '../../data/strategies.json');
  const strategyRaw = await fs.readFile(strategyPath, 'utf-8');
  const strategies = JSON.parse(strategyRaw);
  const strategy = strategies.find((s: any) => s.strategy_id === strategyId);
  if (!strategy) throw new Error('Strategy not found');

  const assetPath = path.join(__dirname, `../../data/assets_${strategyId}.csv`);
  const pricePath = path.join(__dirname, `../../data/prices_${strategy.security}.csv`);
  const prices = await parseCsvFile(pricePath);

  const dateSeq = getDateSequence(strategy.start_date, strategy.interval).map(dateString => new Date(dateString));
  const holdings = await computeHoldings(/*userId = */"default", strategy.security, dateSeq);
  let initialExpectValue = -1;

  const result = dateSeq.map((date, idx) => {
    const priceEntry = prices.find(p => p[0] === date.toISOString().slice(0, 10));
    const price = priceEntry ? parseFloat(priceEntry[1]) : 0;

    const actualValue = holdings[idx][0] * price;

    if (initialExpectValue == -1) {
      initialExpectValue = actualValue;
    }
    const expectedValue = idx * strategy.increment + initialExpectValue;

    return { date, actualValue, expectedValue };
  });

  return result;
}