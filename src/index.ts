import express from 'express';
import { isbot } from 'isbot';
import ky from 'ky';
import slug from 'slug';

import { env } from '@/env';
import { log } from '@/pino';
import { redis } from '@/redis';
import { getCachedContentUrl, setCachedContentUrl } from '@/utils/cache';
import { extractJsonLdScripts, parseJsonLd } from '@/utils/jsonLd';
import { getFullUrl } from '@/utils/medal';

const app = express();

app.disable('x-powered-by');

app.get('/', (_req, res) =>
  res.redirect('https://github.com/ari-party/xmedal.tv'),
);

app.get('/health', (_req, res) => {
  if (redis.status === 'ready') res.status(200).write('OK');
  else res.status(503).write('NOT OK');

  res.end();
});

app.get('/*splat', async (req, res) => {
  const path = req.path.replace(/^\//, '');
  const key = slug(path);
  const fullUrl = getFullUrl(path);

  if (env.NODE_ENV === 'development' || isbot(req.get('user-agent'))) {
    try {
      const start = performance.now();

      let contentUrl = await getCachedContentUrl(key);
      if (!contentUrl) {
        const response = await ky.get(fullUrl);
        if (!response.ok)
          switch (response.status) {
            case 404:
              return res.status(404).end();
            default:
              return;
          }

        const html = await response.text();
        const [jsonLdString] = extractJsonLdScripts(html);
        if (!jsonLdString) return res.status(404).end();

        const jsonLd = parseJsonLd(jsonLdString);
        if (!jsonLd) return;
        if (jsonLd['@type'] !== 'VideoObject') return;
        if (!jsonLd.contentUrl || typeof jsonLd.contentUrl !== 'string') return;

        contentUrl = jsonLd.contentUrl as string;

        await setCachedContentUrl(key, contentUrl);
      }

      const end = performance.now();
      res.setHeader('x-medal-lookup', `${((end - start) / 1_000).toFixed(4)}s`);

      res.redirect(contentUrl);
    } catch (err) {
      log.error(err);
    } finally {
      if (!res.headersSent) res.status(500).end();
    }
  }

  res.redirect(fullUrl);
});

app.listen(env.PORT, '0.0.0.0', () =>
  log.info(`Server listening on 0.0.0.0:${env.PORT}`),
);
