import { extractJsonLdScripts, parseJsonLd } from '@/utils/jsonLd';

export function getFullUrl(path: string) {
  return `https://medal.tv/${path}`;
}

export function extractContentUrl(html: string) {
  const [jsonLdString] = extractJsonLdScripts(html);
  if (!jsonLdString) return;

  const jsonLd = parseJsonLd(jsonLdString);
  if (!jsonLd) return;
  if (jsonLd['@type'] !== 'VideoObject') return;
  if (typeof jsonLd.contentUrl !== 'string') return;

  return jsonLd.contentUrl as string;
}
