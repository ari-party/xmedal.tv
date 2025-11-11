import { createEnv } from '@t3-oss/env-core';
import { z } from 'zod';

export const env = createEnv({
  server: {
    NODE_ENV: z.string().default('development'),

    PORT: z
      .string()
      .default('3000')
      .transform((v) => parseInt(v, 10))
      .pipe(z.number()),

    REDIS_URL: z.string().url().default('redis://localhost:6379'),
  },
  runtimeEnv: process.env,
  emptyStringAsUndefined: true,
});
