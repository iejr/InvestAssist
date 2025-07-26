import fs from 'fs/promises';
import path from 'path';
import { v4 as uuidv4 } from 'uuid';
import { appendCsvRow } from '../utils/file';

const STRATEGY_FILE = path.join(__dirname, '../../data/strategies.json');

export async function createStrategy({ name, start_date, interval, increment, security }: any) {
  const strategy_id = uuidv4();
  const newStrategy = { strategy_id, name, start_date, interval, increment, security };
  const raw = await fs.readFile(STRATEGY_FILE, 'utf-8').catch(() => '[]');
  const strategies = JSON.parse(raw);
  strategies.push(newStrategy);
  await fs.writeFile(STRATEGY_FILE, JSON.stringify(strategies, null, 2));
  return newStrategy;
}

export async function addAsset(strategy_id: string, { date, shares }: any) {
  const filePath = path.join(__dirname, `../../data/assets_${strategy_id}.csv`);
  await appendCsvRow(filePath, [date, shares]);
}
