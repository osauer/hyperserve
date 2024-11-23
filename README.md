# hs 🚀

_A simple, dependable, and dependency-free HTTP srv for the hyperspeed age._

hs is a Go-based HTTP srv designed to handle requests, serve responses, and enforce rate limits—all while avoiding
external dependencies. With default configurations, clean middleware handling, and basic endpoint checks, hs is ready to
roll with just a few lines of Go.

## Features

- **Middleware Support**: Easily add and chain middleware functions to extend srv functionality.
- **Rate Limiting**: Configure requests per second and burst limits to keep things under control.
- **Configurable**: Load configurations from environment variables, a JSON file, or rely on built-in defaults.
- **Health Checks**: `/healthz`, `/readyz`, and `/livez` endpoints for easy liveness and readiness probing.
- **Zero External Dependencies**: Because who needs them, really?

## Getting Started

## Support

Visit, HYP, an AI powered support agent to help building on top of hs (or enhance
it) : https://chatgpt.com/g/g-OUyPwXtsN-hyp

### Requirements

- **Go 1.23.2+**: hs is crafted for the latest features in Go 1.23 and up.

### Installation

Clone the repository to your local machine:

```sh
git clone https://github.com/osauer/hs.git
cd hs