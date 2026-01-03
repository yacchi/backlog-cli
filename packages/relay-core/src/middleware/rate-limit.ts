/**
 * Rate limiting middleware.
 */

import type { RateLimitConfig, CacheProvider } from "../config/types.js";

/**
 * Token bucket rate limiter.
 */
export class RateLimiter {
  private requestsPerMinute: number;
  private burstSize: number;
  private cache?: CacheProvider;

  constructor(config?: RateLimitConfig, cache?: CacheProvider) {
    this.requestsPerMinute = config?.requests_per_minute ?? 60;
    this.burstSize = config?.burst_size ?? 10;
    this.cache = cache;
  }

  /**
   * Check if a request should be allowed.
   * @param key The rate limit key (typically client IP)
   * @returns true if allowed, false if rate limited
   */
  async isAllowed(key: string): Promise<boolean> {
    if (!this.cache) {
      // No cache provider, allow all requests
      return true;
    }

    const cacheKey = `ratelimit:${key}`;
    const now = Date.now();
    const windowMs = 60000; // 1 minute window

    const data = await this.cache.get(cacheKey);
    let tokens: number;
    let lastRefill: number;

    if (data) {
      const parsed = JSON.parse(data);
      tokens = parsed.tokens;
      lastRefill = parsed.lastRefill;

      // Calculate tokens to add based on time passed
      const timePassed = now - lastRefill;
      const tokensToAdd = Math.floor(
        (timePassed / windowMs) * this.requestsPerMinute
      );

      tokens = Math.min(this.burstSize, tokens + tokensToAdd);
    } else {
      tokens = this.burstSize;
      lastRefill = now;
    }

    if (tokens < 1) {
      return false;
    }

    // Consume a token
    tokens -= 1;

    // Save state
    await this.cache.set(
      cacheKey,
      JSON.stringify({ tokens, lastRefill: now }),
      60 // TTL of 60 seconds
    );

    return true;
  }

  /**
   * Get the rate limit headers for a response.
   */
  getHeaders(remaining: number): Record<string, string> {
    return {
      "X-RateLimit-Limit": String(this.requestsPerMinute),
      "X-RateLimit-Remaining": String(Math.max(0, remaining)),
      "X-RateLimit-Reset": String(Math.ceil(Date.now() / 1000) + 60),
    };
  }
}
