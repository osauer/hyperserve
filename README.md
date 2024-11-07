# hyperserve 🚀

_A simple, dependable, and dependency-free HTTP server for the hyperspeed age._

hyperserve is a Go-based HTTP server designed to handle requests, serve responses, and enforce rate limits—all while avoiding external dependencies. With default configurations, clean middleware handling, and basic endpoint checks, hyperserve is ready to roll with just a few lines of Go.

## Features

- **Middleware Support**: Easily add and chain middleware functions to extend server functionality.
- **Rate Limiting**: Configure requests per second and burst limits to keep things under control.
- **Configurable**: Load configurations from environment variables, a JSON file, or rely on built-in defaults.
- **Health Checks**: `/healthz`, `/readyz`, and `/livez` endpoints for easy liveness and readiness probing.
- **Zero External Dependencies**: Because who needs them, really?

## Getting Started

### Requirements

- **Go 1.23.2+**: hyperserve is crafted for the latest features in Go 1.23 and up.

### Installation

Clone the repository to your local machine:

```sh
git clone https://github.com/yourusername/hyperserve.git
cd hyperserve
