import express from 'express';
import { getHistory } from '../services/historyService';
const router = express.Router();

router.get('/:strategyId', async (req, res) => {
  const data = await getHistory(req.params.strategyId);
  res.json(data);
});

export default router;