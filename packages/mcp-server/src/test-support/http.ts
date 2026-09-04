/**
 * Test-only helper for typing `fetch`/Hono `Response` bodies.
 *
 * `Response#json()` is typed as `Promise<unknown>` under `strict`. Tests know
 * the shape of the body they expect back, so this gives them a single place
 * to say so instead of scattering `as any` / `as Record<string, any>` casts.
 */
export async function readJson<T = Record<string, unknown>>(res: Response): Promise<T> {
    return (await res.json()) as T;
}
