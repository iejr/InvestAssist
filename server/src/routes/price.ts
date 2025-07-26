import express from 'express';
import { appendPrices } from '../services/priceService';
const router = express.Router();

router.post('/SPY500', async (req, res) => {
  await appendPrices('SPY500', req.body);
  res.sendStatus(200);
});

export default router;