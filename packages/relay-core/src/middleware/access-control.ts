/**
 * Access control middleware for space and project restrictions.
 */

import type { AccessControlConfig } from "../config/types.js";

/**
 * Access control checker.
 */
export class AccessControl {
  private allowedSpacePatterns: string[];
  private allowedProjectPatterns: string[];

  constructor(config?: AccessControlConfig) {
    this.allowedSpacePatterns = this.parsePatterns(
      config?.allowedSpacePatterns
    );
    this.allowedProjectPatterns = this.parsePatterns(
      config?.allowedProjectPatterns
    );
  }

  private parsePatterns(patterns?: string): string[] {
    if (!patterns) {
      return [];
    }
    return patterns
      .split(";")
      .map((p) => p.trim())
      .filter((p) => p.length > 0);
  }

  /**
   * Check if a space is allowed.
   * @throws Error if the space is not allowed
   */
  checkSpace(space: string): void {
    if (this.allowedSpacePatterns.length === 0) {
      // No restrictions
      return;
    }

    if (!this.matchesAnyPattern(space, this.allowedSpacePatterns)) {
      throw new Error(`Space '${space}' is not allowed`);
    }
  }

  /**
   * Check if a project is allowed.
   * @throws Error if the project is not allowed
   */
  checkProject(project?: string): void {
    if (!project) {
      // No project specified is always allowed
      return;
    }

    if (this.allowedProjectPatterns.length === 0) {
      // No restrictions
      return;
    }

    if (!this.matchesAnyPattern(project, this.allowedProjectPatterns)) {
      throw new Error(`Project '${project}' is not allowed`);
    }
  }

  /**
   * Check if a value matches any of the given patterns.
   * Patterns support * as a wildcard for any characters.
   */
  private matchesAnyPattern(value: string, patterns: string[]): boolean {
    for (const pattern of patterns) {
      if (this.matchPattern(value, pattern)) {
        return true;
      }
    }
    return false;
  }

  /**
   * Match a value against a pattern.
   * Supports * as a wildcard for any characters.
   */
  private matchPattern(value: string, pattern: string): boolean {
    // Convert glob pattern to regex
    const regexPattern = pattern
      .replace(/[.+^${}()|[\]\\]/g, "\\$&") // Escape special regex chars
      .replace(/\*/g, ".*") // Convert * to .*
      .replace(/\?/g, "."); // Convert ? to .

    const regex = new RegExp(`^${regexPattern}$`);
    return regex.test(value);
  }
}
