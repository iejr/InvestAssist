import { addDays, format, parseISO } from 'date-fns';

export function getDateSequence(start: string, interval: 'weekly' | 'monthly'): string[] {
  const result: string[] = [];
  let date = parseISO(start);
  const now = new Date();

  while (date <= now) {
    result.push(format(date, 'yyyy-MM-dd'));
    date = addDays(date, interval === 'weekly' ? 7 : 30);
  }

  return result;
}

export function getIntervalCycles(start: Date, end: Date, interval: 'weekly' | 'monthly'): number {
  const diff = end.getTime() - start.getTime();
  const dayTime = 24 * 60 * 60 * 1000;
  const weekTime = 7 * dayTime;
  const monthTime = 30 * dayTime;

  const cycles = interval === 'weekly' ? Math.floor(diff / weekTime) : Math.floor(diff / monthTime);
  return cycles;
}