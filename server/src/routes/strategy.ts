import express from 'express';
import { getAllStrategies, createStrategy, addAsset } from '../services/strategyService';
const router = express.Router();

router.get('/', async (req, res) => {
  const strategies = await getAllStrategies();
  res.json(strategies);
});

router.post('/', async (req, res) => {
  const strategy = await createStrategy(req.body);
  res.json(strategy);
});

router.post('/:id/asset', async (req, res) => {
  const { id } = req.params;
  await addAsset(id, req.body);
  res.sendStatus(200);
});

export default router;
