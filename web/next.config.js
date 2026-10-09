/** @type {import('next').NextConfig} */
module.exports = {
  output: 'standalone',
  outputFileTracingRoot: __dirname,
  poweredByHeader: false,
  images: { unoptimized: true },
  allowedDevOrigins: ['127.0.0.1'],
  async headers() {
    // The UI renders user-supplied URLs and remote ogImage/favicon values.
    // A full CSP would need per-page nonces for Next's inline runtime; these
    // headers are the safe subset that cannot break rendering.
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
