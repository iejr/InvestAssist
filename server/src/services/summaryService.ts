import fs from 'fs/promises';
import path from 'path';
import { parseCsvFile } from '../utils/file';
import { getIntervalCycles } from '../utils/date';
import { computeHoldings } from './transactionService';

export async function getSummary(strategyId: string) {

  const strategyPath = path.join(__dirname, '../../data/strategies.json');
  const strategyRaw = await fs.readFile(strategyPath, 'utf-8');
  const strategies = JSON.parse(strategyRaw);
  const strategy = strategies.find((s: any) => s.strategy_id === strategyId);
  if (!strategy) throw new Error('Strategy not found');

  const pricePath = path.join(__dirname, `../../data/prices_${strategy.security}.csv`);
  const prices = await parseCsvFile(pricePath);

  const dateStart = new Date(strategy.start_date);
  const dateUntil = new Date();
  const cycleNum = getIntervalCycles(dateStart, dateUntil, strategy.interval);
  const holdings = await computeHoldings(/*userId = */"default", strategy.security, [dateStart, dateUntil]);

  if (holdings.length < 2) {
    // something wrong
    return;
  }

  const [initialHolding, _] = holdings[0];
  const [holding, cost] = holdings[1];

  let initialExpectValue = -1;
  {
    const priceEntry = prices.find(p => p[0] === dateStart.toISOString().slice(0, 10));
    const price = priceEntry ? parseFloat(priceEntry[1]) : 0;
    initialExpectValue = initialHolding * Number(price);
  }

  const priceEntry = prices.find(p => p[0] === dateUntil.toISOString().slice(0, 10));
  const price = priceEntry ? parseFloat(priceEntry[1]) : 0;

  const actualValue = holding * price;
  const expectedValue = cycleNum * strategy.increment + initialExpectValue;
  const nextInvestment = expectedValue - actualValue;

  return {
    current_value: actualValue,
    total_cash_invested: cost,
    gain_loss: actualValue - cost,
    next_investment: nextInvestment,
  };
}