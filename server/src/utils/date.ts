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