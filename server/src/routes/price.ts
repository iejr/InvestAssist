import express from 'express';
import { appendPrices } from '../services/priceService';
import { isValidSecurity } from '../utils/security';
const router = express.Router();

router.post('/:security', async (req, res) => {
  const { security } = req.params;
  if (!isValidSecurity(security)) {
    return res.status(400).json({ error: 'Unsupported security' });
  }

  const prices = req.body; // [{ date, price }]
  await appendPrices(security, prices);
  res.sendStatus(200);
});

export default router;