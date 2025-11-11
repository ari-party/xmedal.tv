import { redis } from '@/redis';

export async function getCachedContentUrl(key: string) {
  const result = await redis.get(key);
  return result || null;
}

export async function setCachedContentUrl(key: string, value: string) {
  await redis.setex(key, 60 * 60 * 12, value);
}
