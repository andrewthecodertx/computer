/** @type {import('next').NextConfig} */
module.exports = {
  output: 'standalone',
  outputFileTracingRoot: __dirname,
  poweredByHeader: false,
  images: { unoptimized: true },
  allowedDevOrigins: ['127.0.0.1'],
  async headers() {
    // The UI renders user-supplied URLs and remote ogImage/favicon values.
    // The Content-Security-Policy (nonce-based, review CR-10) is emitted per
    // response by web/middleware.ts so Next's inline bootstrap scripts carry
    // the nonce; these headers are the static non-CSP subset.
    return [
      {
        source: '/:path*',
        headers: [
          { key: 'X-Content-Type-Options', value: 'nosniff' },
          { key: 'X-Frame-Options', value: 'DENY' },
          { key: 'Referrer-Policy', value: 'strict-origin-when-cross-origin' },
          { key: 'Permissions-Policy', value: 'camera=(), microphone=(), geolocation=(), interest-cohort=()' },
        ],
      },
    ]
  },
}
