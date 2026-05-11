# OneAppFactory Web

This directory contains the web launcher for OneAppFactory.
It provides the AppFactory job console, configuration screen, and embedded frontend served by the OneAppFactory launcher binary.

## Architecture

The service is structured as a monorepo containing both the backend and frontend code to simplify the launcher build.

*   **`backend/`**: The importable Go web server. It provides AppFactory REST APIs and serves the compiled frontend assets from the launcher executable.
*   **`frontend/`**: The Vite + React + TanStack Router single-page application (SPA). It provides the interactive user interface.

## Getting Started

### Prerequisites

*   Go 1.25+
*   Node.js 20+ with pnpm

### Development

Run both the frontend dev server and the Go backend simultaneously:

```bash
make dev
```

Or run them separately:

```bash
make dev-frontend   # Vite dev server
make dev-backend    # Go backend
```

### Build

Build the frontend and embed it into a single Go binary:

```bash
make build
```

The output binary is `build/oneappfactory-launcher`.

### Other Commands

```bash
make test    # Run backend tests and frontend lint
make lint    # Run go vet and prettier/eslint
make clean   # Remove all build artifacts
```
