const REGEX = /<script type="application\/ld\+json">([\s\S]*?)<\/script>/g;

export function extractJsonLdScripts(htmlString: string): string[] {
  const matches: string[] = [];

  let match = REGEX.exec(htmlString);

  while (match !== null) {
    if (match[1]) matches.push(match[1]);

    match = REGEX.exec(htmlString);
  }

  return matches;
}

export function parseJsonLd(jsonLd: string) {
  try {
    const parsed = JSON.parse(jsonLd);
    return parsed;
  } catch (_) {
    return null;
  }
}
