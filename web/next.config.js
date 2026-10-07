// Next.js config for the computer front-end, now backed by the Go API.
//
// The app calls relative /api/* paths, so the Next server proxies them to the
// Go backend (API_BASE_URL). /api/auth/* stays local — NextAuth's session,
// csrf and callback handlers live here, not in Go.
//
//   API_BASE_URL=http://go-computer:8080     # Docker DNS
//   (unset in dev → no rewrite, app talks to its own API routes)

const path = require('path');

const nextConfig = {
  distDir: process.env.NEXT_DIST_DIR || '.next',
  output: process.env.NEXT_OUTPUT_MODE || 'standalone',
  poweredByHeader: false,
  productionBrowserSourceMaps: false,
  outputFileTracingRoot: process.env.NEXT_OUTPUT_MODE ? path.join(__dirname, '../') : __dirname,
  typescript: {
    ignoreBuildErrors: true,
  },
  images: { unoptimized: true },
  async rewrites() {
    // Proxy /api/* to the Go backend (API_BASE_URL), except /api/auth/* which
    // stays local — NextAuth's session, csrf and callback handlers live here.
    // Only active when API_BASE_URL is set; useful once the Go handlers are
    // implemented and the local Next routes are removed.
    const apiBase = process.env.API_BASE_URL;
    if (!apiBase) return [];
    return [
      {
        source: '/api/:path((?!auth/).*).',
        destination: `${apiBase}/api/:path*`,
      },
    ];
  },
  async headers() {
    if (process.env.NODE_ENV !== 'development') return [];
    return [{ source: '/_next/static/:path*', headers: [{ key: 'Cache-Control', value: 'no-store, must-revalidate' }] }];
  },
  allowedDevOrigins: ['127.0.0.1', '8a671ebdc.na116.preview.abacusai.app'],
};

const fs = require('fs');
const userConfigPath = path.join(__dirname, 'next.config.user.json');
const userConfigAllowedKeys = { skipTrailingSlashRedirect: 'boolean', trailingSlash: 'boolean' };
if (fs.existsSync(userConfigPath)) {
  const userConfig = JSON.parse(fs.readFileSync(userConfigPath, 'utf8'));
  for (const key of Object.keys(userConfig)) {
    if (typeof userConfig[key] !== userConfigAllowedKeys[key]) {
      throw new Error(`next.config.user.json: unsupported override "${key}". Supported boolean keys: skipTrailingSlashRedirect, trailingSlash.`);
    }
    nextConfig[key] = userConfig[key];
  }
}

module.exports = nextConfig;