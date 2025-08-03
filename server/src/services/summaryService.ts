import fs from 'fs/promises';
import path from 'path';
import { parseCsvFile } from '../utils/file';
import { getDateSequence } from '../utils/date';

export async function getSummary(strategyId: string) {
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
  let initialExpectValue = -1;
  let nextInvestment = 0;

  const result = dateSeq.map(date => {
    const assetEntry = assets.find(a => a[0] === date);
    if (assetEntry) totalShares = parseFloat(assetEntry[1]);

    const priceEntry = prices.find(p => p[0] === date);
    const price = priceEntry ? parseFloat(priceEntry[1]) : 0;

    const actualValue = totalShares * price;
    if (initialExpectValue == -1) {
      initialExpectValue = actualValue;
    }
    const intervalCount = dateSeq.indexOf(date);
    const expectedValue = intervalCount * strategy.increment + initialExpectValue;
    nextInvestment = expectedValue - actualValue;
  });

  return {
    current_value: -1,
    total_cash_invested: -1,
    gain_loss: 0,
    next_investment: nextInvestment,
  };
}