/**
 * Resolve the image tag to deploy from the container registry.
 *
 * When the deployer does not pin an explicit tag, the latest semver tag is
 * selected from the registry's tag list at synth time. Stable releases and
 * prereleases are kept in separate buckets so a `0.19.1` release is never
 * shadowed by — nor accidentally replaced with — its `0.19.1-rc.1` prerelease.
 *
 * Registry API: the standard OCI distribution `/v2/<repo>/tags/list` with an
 * anonymous bearer token. Verified against GHCR; other registries that follow
 * the same token + tags/list flow work too.
 */

import * as semver from "semver";

export interface ResolveImageTagOptions {
  /**
   * When true, select the highest *prerelease* tag (to target a dev build).
   * When false/omitted, select the highest *stable* release.
   */
  prerelease?: boolean;
}

interface ParsedRef {
  registry: string;
  repository: string;
}

/** Split `ghcr.io/owner/name` into registry + repository. */
export function parseImageRef(source: string): ParsedRef {
  const slash = source.indexOf("/");
  if (slash < 0) {
    throw new Error(
      `Invalid image source (expected "<registry>/<repository>"): ${source}`,
    );
  }
  return {
    registry: source.slice(0, slash),
    repository: source.slice(slash + 1),
  };
}

async function fetchAnonymousToken(
  registry: string,
  repository: string,
): Promise<string | undefined> {
  try {
    const resp = await fetch(
      `https://${registry}/token?service=${registry}&scope=repository:${repository}:pull`,
    );
    if (!resp.ok) {
      return undefined;
    }
    const body = (await resp.json()) as { token?: string };
    return body.token;
  } catch {
    return undefined;
  }
}

/** List all tags for an image repository. */
export async function fetchImageTags(source: string): Promise<string[]> {
  const { registry, repository } = parseImageRef(source);
  const token = await fetchAnonymousToken(registry, repository);
  const headers: Record<string, string> = {};
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }
  const resp = await fetch(`https://${registry}/v2/${repository}/tags/list`, {
    headers,
  });
  if (!resp.ok) {
    throw new Error(
      `Failed to list tags for ${source}: ${resp.status} ${resp.statusText}. ` +
        `Is the image published and public?`,
    );
  }
  const body = (await resp.json()) as { tags?: string[] };
  return body.tags ?? [];
}

/**
 * Resolve the latest semver tag for an image repository.
 *
 * @throws if no matching tag exists (e.g. prerelease requested but none exist).
 */
export async function resolveLatestImageTag(
  source: string,
  options?: ResolveImageTagOptions,
): Promise<string> {
  const wantPrerelease = options?.prerelease ?? false;
  const tags = await fetchImageTags(source);

  const candidates = tags.filter((t) => {
    if (!semver.valid(t)) {
      return false;
    }
    const isPrerelease = (semver.prerelease(t)?.length ?? 0) > 0;
    return wantPrerelease ? isPrerelease : !isPrerelease;
  });

  if (candidates.length === 0) {
    throw new Error(
      `No ${wantPrerelease ? "prerelease" : "stable"} semver tag found for ${source}. ` +
        `Available tags: ${tags.join(", ") || "(none)"}. ` +
        (wantPrerelease
          ? "Set image.tag explicitly, or publish a prerelease."
          : "Set image.tag explicitly, set image.prerelease=true to target a dev build, or publish a stable release."),
    );
  }

  // Highest version first.
  candidates.sort(semver.rcompare);
  return candidates[0];
}
