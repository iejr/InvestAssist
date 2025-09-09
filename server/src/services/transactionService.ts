import fs from 'fs';
import path from 'path';
import { appendCsvRows, parseCsvFile, parseCsvFileAlter } from '../utils/file';
import { transactionModel } from '../model/transactionModel';

const DATA_DIR = path.join(__dirname, '../data/transactions');

function getFilePath(userId: string) {
  return path.join(DATA_DIR, `user-${userId}.csv`);
}

export async function loadTransactions(userId: string, securityId?: string): Promise<transactionModel[]> {
  const file = getFilePath(userId);
  if (!fs.existsSync(file)) return [];
  const contents = await parseCsvFileAlter<transactionModel>(file);

  if (securityId) {
    return contents.filter(e => e.security == securityId)
  }
  else {
    return contents;
  }
}

export function saveTransactions(userId: string, transactions: Array<Array<string>>) {
  const file = getFilePath(userId);
  appendCsvRows(file, transactions);
}

// export async function addTransaction(userId: string, tx: Array<string>) {
//   const transactions = await loadTransactions(userId);
//   transactions.push(tx);
//   saveTransactions(userId, transactions);
// }

export async function computeHoldings(userId: string, securityId: string, dates: Date[]): Promise<Array<number>> {
  const transactions = await loadTransactions(userId, securityId);
  const holdings: Record<string, number> = {};
  const snapshots: number[] = [];

  dates.sort((e1, e2) => e1.getTime() - e2.getTime());

  let dateIdx = 0;
  let porfolio = 0;
  for (const t of transactions) {
    if (dateIdx > dates.length) break;

    const txDate = t.timestamp;
    while (dates[dateIdx] < txDate) {
      snapshots.push(porfolio);
      dateIdx++;
    }

    const shares = t.type === 'BUY' ? t.shares : -t.shares;
    porfolio += shares * t.price;
  }

  while (dateIdx < dates.length) {
    snapshots.push(porfolio);
  }

  return snapshots;
}