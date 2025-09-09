export interface transactionModel {
    timestamp: Date;
    security: string;
    type: 'BUY' | 'SELL';
    shares: number;
    price: number;
    notes: string;
};