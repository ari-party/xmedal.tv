import Redis from 'ioredis';

import { env } from '@/env';
import { log } from '@/pino';

export const redis = new Redis(env.REDIS_URL);

redis.on('connecting', () => log.info({ name: 'Redis' }, 'Connecting'));
redis.on('connect', () => log.info({ name: 'Redis' }, 'Connected'));
redis.on('error', (error) => log.error({ ...error, name: 'Redis' }, 'Error'));
