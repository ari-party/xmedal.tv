import pino from 'pino';

import { env } from '@/env';

import type { PrettyOptions } from 'pino-pretty';

export const log = pino({
  base: undefined,
  level: env.NODE_ENV === 'development' ? 'debug' : 'info',
  ...(env.NODE_ENV === 'development' && {
    transport: {
      target: 'pino-pretty',
      options: {
        colorize: false,
      } as PrettyOptions,
    },
  }),
});
