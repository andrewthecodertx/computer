/** @type {import('next').NextConfig} */
module.exports = {
  output: 'standalone',
  outputFileTracingRoot: __dirname,
  poweredByHeader: false,
  images: { unoptimized: true },
  allowedDevOrigins: ['127.0.0.1'],
}
