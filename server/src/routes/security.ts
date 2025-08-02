import express from 'express';
import { ALLOWED_SECURITIES } from '../utils/security';

const router = express.Router();
router.get('/', (req, res) => {
  res.json(ALLOWED_SECURITIES);
});
export default router;