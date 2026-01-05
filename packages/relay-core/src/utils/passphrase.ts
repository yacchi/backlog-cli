/**
 * Passphrase verification utilities.
 */

import bcrypt from "bcryptjs";

/**
 * Verify a passphrase against a bcrypt hash.
 */
export async function verifyPassphrase(
  hash: string,
  passphrase: string
): Promise<boolean> {
  try {
    return await bcrypt.compare(passphrase, hash);
  } catch {
    console.error("[verifyPassphrase] Failed to compare passphrase");
    return false;
  }
}
