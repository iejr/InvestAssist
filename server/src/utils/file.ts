import fs from 'fs/promises';

export async function appendCsvRow(filePath: string, row: Array<string | number>) {
  const line = row.join(',') + '\n';
  await fs.appendFile(filePath, line, 'utf-8');
}

export async function appendCsvRows(filePath: string, rows: Array<Array<string | number>>) {
  const content = rows.map(row => row.join(',')).join('\n') + '\n';
  await fs.appendFile(filePath, content, 'utf-8');
}

export async function parseCsvFile(filePath: string): Promise<string[][]> {
  const raw = await fs.readFile(filePath, 'utf-8');
  return raw.trim().split('\n').map(line => line.split(','));
}