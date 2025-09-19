export interface transactionModel {
    timestamp: string;
    security: string;
    type: 'BUY' | 'SELL';
    shares: number;
    price: number;
    notes: string;
};