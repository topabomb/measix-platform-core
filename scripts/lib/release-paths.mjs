import { resolve } from 'node:path'

// Release packaging and contract verification must consume the same checkout.
export function releaseAndroidRoot(coreRoot, env = process.env) {
  return resolve(env.MEASIX_RELEASE_ANDROID_ROOT || resolve(coreRoot, '..', '..', 'rikkahub_mcp'))
}
