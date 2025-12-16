import { createMDX } from "fumadocs-mdx/next"

/** @type {import('next').NextConfig} */
const config = {
  output: 'export',
  basePath: '/evm',
  reactStrictMode: true,
  typescript: {
    ignoreBuildErrors: true,
  },
  experimental: {
    webpackBuildWorker: true,
  },
  images: {
    unoptimized: true,
  },
}

const withMDX = createMDX()

export default withMDX(config)
