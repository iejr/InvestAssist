import express from 'express';
import { getSummary } from '../services/summaryService';

const router = express.Router();

router.get('/:id', async (req, res) => {
  try {
    const summary = await getSummary(req.params.id);
    res.json(summary);
  } catch (err: any) {
    res.status(500).json({ error: err.message });
  }
});

export default router;