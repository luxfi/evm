import Link from "next/link";

export default function HomePage() {
  return (
    <main className="flex flex-1 flex-col">
      {/* Hero Section */}
      <section className="relative overflow-hidden border-b fd-border">
        <div className="absolute inset-0 bg-gradient-to-br from-fd-primary/5 via-transparent to-fd-primary/10" />
        <div className="container relative mx-auto px-6 py-24 text-center md:py-32">
          <div className="mx-auto max-w-3xl">
            <h1 className="mb-6 text-4xl font-bold tracking-tight fd-foreground md:text-6xl">
              Lux EVM
            </h1>
            <p className="mb-8 text-lg text-fd-muted-foreground md:text-xl">
              The EVM compatibility layer for Lux Network. Build, deploy, and scale
              smart contracts with post-quantum security, native precompiles, and
              seamless cross-chain messaging.
            </p>
            <div className="flex flex-col items-center justify-center gap-4 sm:flex-row">
              <Link
                href="/docs"
                className="inline-flex h-11 items-center justify-center rounded-md bg-fd-primary px-8 text-sm font-medium text-fd-primary-foreground transition-colors hover:bg-fd-primary/90"
              >
                Get Started
              </Link>
              <Link
                href="https://github.com/luxfi/evm"
                className="inline-flex h-11 items-center justify-center rounded-md border fd-border bg-fd-background px-8 text-sm font-medium fd-foreground transition-colors hover:bg-fd-accent"
              >
                View on GitHub
              </Link>
            </div>
          </div>
        </div>
      </section>

      {/* Quick Install Section */}
      <section className="border-b fd-border bg-fd-card/50">
        <div className="container mx-auto px-6 py-12">
          <div className="mx-auto max-w-2xl text-center">
            <h2 className="mb-4 text-sm font-semibold uppercase tracking-wider text-fd-muted-foreground">
              Quick Install
            </h2>
            <div className="relative">
              <pre className="overflow-x-auto rounded-lg border fd-border bg-fd-background p-4 text-left">
                <code className="text-sm fd-foreground">
                  forge install luxfi/evm
                </code>
              </pre>
            </div>
            <p className="mt-4 text-sm text-fd-muted-foreground">
              Add to your Foundry project with a single command
            </p>
          </div>
        </div>
      </section>

      {/* Features Grid */}
      <section className="border-b fd-border">
        <div className="container mx-auto px-6 py-20">
          <div className="mb-12 text-center">
            <h2 className="mb-4 text-3xl font-bold fd-foreground">
              Built for Performance
            </h2>
            <p className="mx-auto max-w-2xl text-fd-muted-foreground">
              Everything you need to build high-performance decentralized applications
              on Lux Network
            </p>
          </div>
          <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
            {/* Smart Contracts */}
            <div className="rounded-lg border fd-border bg-fd-card p-6 transition-colors hover:bg-fd-accent/50">
              <div className="mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-fd-primary/10">
                <svg
                  className="h-6 w-6 text-fd-primary"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"
                  />
                </svg>
              </div>
              <h3 className="mb-2 text-lg font-semibold fd-foreground">
                Smart Contracts
              </h3>
              <p className="text-sm text-fd-muted-foreground">
                Full Solidity 0.8.24 support with optimized compilation. Deploy
                existing Ethereum contracts with zero modifications.
              </p>
            </div>

            {/* Precompiles */}
            <div className="rounded-lg border fd-border bg-fd-card p-6 transition-colors hover:bg-fd-accent/50">
              <div className="mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-fd-primary/10">
                <svg
                  className="h-6 w-6 text-fd-primary"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z"
                  />
                </svg>
              </div>
              <h3 className="mb-2 text-lg font-semibold fd-foreground">
                Native Precompiles
              </h3>
              <p className="text-sm text-fd-muted-foreground">
                Hardware-accelerated cryptography including ML-DSA, FROST, CGGMP21,
                and post-quantum Ringtail signatures.
              </p>
            </div>

            {/* Cross-Chain */}
            <div className="rounded-lg border fd-border bg-fd-card p-6 transition-colors hover:bg-fd-accent/50">
              <div className="mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-fd-primary/10">
                <svg
                  className="h-6 w-6 text-fd-primary"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4"
                  />
                </svg>
              </div>
              <h3 className="mb-2 text-lg font-semibold fd-foreground">
                Cross-Chain Messaging
              </h3>
              <p className="text-sm text-fd-muted-foreground">
                Warp messaging enables trustless communication between Lux chains
                and external networks with BLS verification.
              </p>
            </div>

            {/* Gas Management */}
            <div className="rounded-lg border fd-border bg-fd-card p-6 transition-colors hover:bg-fd-accent/50">
              <div className="mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-fd-primary/10">
                <svg
                  className="h-6 w-6 text-fd-primary"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M13 10V3L4 14h7v7l9-11h-7z"
                  />
                </svg>
              </div>
              <h3 className="mb-2 text-lg font-semibold fd-foreground">
                Gas Management
              </h3>
              <p className="text-sm text-fd-muted-foreground">
                Dynamic fee management with configurable base fees, priority fees,
                and native token minting controls.
              </p>
            </div>

            {/* Tooling */}
            <div className="rounded-lg border fd-border bg-fd-card p-6 transition-colors hover:bg-fd-accent/50">
              <div className="mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-fd-primary/10">
                <svg
                  className="h-6 w-6 text-fd-primary"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
                  />
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
                  />
                </svg>
              </div>
              <h3 className="mb-2 text-lg font-semibold fd-foreground">
                Developer Tooling
              </h3>
              <p className="text-sm text-fd-muted-foreground">
                First-class Foundry support with deployment scripts, testing
                utilities, and comprehensive documentation.
              </p>
            </div>

            {/* Security */}
            <div className="rounded-lg border fd-border bg-fd-card p-6 transition-colors hover:bg-fd-accent/50">
              <div className="mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-fd-primary/10">
                <svg
                  className="h-6 w-6 text-fd-primary"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"
                  />
                </svg>
              </div>
              <h3 className="mb-2 text-lg font-semibold fd-foreground">
                Post-Quantum Security
              </h3>
              <p className="text-sm text-fd-muted-foreground">
                Future-proof cryptography with ML-DSA (Dilithium), Ringtail
                lattice signatures, and quantum-safe threshold schemes.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* CTA Section */}
      <section className="border-b fd-border bg-fd-card/30">
        <div className="container mx-auto px-6 py-16 text-center">
          <h2 className="mb-4 text-2xl font-bold fd-foreground">
            Ready to Build?
          </h2>
          <p className="mb-8 text-fd-muted-foreground">
            Start building on Lux EVM today with our comprehensive documentation
          </p>
          <Link
            href="/docs"
            className="inline-flex h-11 items-center justify-center rounded-md bg-fd-primary px-8 text-sm font-medium text-fd-primary-foreground transition-colors hover:bg-fd-primary/90"
          >
            Read the Docs
          </Link>
        </div>
      </section>

      {/* Footer */}
      <footer className="border-t fd-border bg-fd-background">
        <div className="container mx-auto px-6 py-8">
          <div className="flex flex-col items-center justify-between gap-4 md:flex-row">
            <div className="flex items-center gap-2">
              <span className="text-lg font-semibold fd-foreground">Lux EVM</span>
              <span className="text-sm text-fd-muted-foreground">
                by Lux Industries
              </span>
            </div>
            <div className="flex items-center gap-6">
              <Link
                href="https://github.com/luxfi/evm"
                className="text-sm text-fd-muted-foreground transition-colors hover:text-fd-foreground"
              >
                GitHub
              </Link>
              <Link
                href="https://lux.network"
                className="text-sm text-fd-muted-foreground transition-colors hover:text-fd-foreground"
              >
                Lux Network
              </Link>
              <Link
                href="/docs"
                className="text-sm text-fd-muted-foreground transition-colors hover:text-fd-foreground"
              >
                Documentation
              </Link>
            </div>
          </div>
          <div className="mt-6 border-t fd-border pt-6 text-center">
            <p className="text-sm text-fd-muted-foreground">
              Built with post-quantum security for the future of decentralized computing
            </p>
          </div>
        </div>
      </footer>
    </main>
  );
}
