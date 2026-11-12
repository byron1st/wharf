/** jiraURL is the browser link of a Jira issue key on the configured site, or null without a site URL. */
export function jiraURL(siteURL: string | undefined, key: string): string | null {
  if (!siteURL) return null
  return `${siteURL.replace(/\/+$/, '')}/browse/${encodeURIComponent(key)}`
}
