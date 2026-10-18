# Contributing

Thanks for wanting to help. LAN Orangutan is a small project with a clear idea of what it is, so this page is short.

## The one rule

LAN Orangutan is meant to be the simplest and fastest scanner to reach for. One binary, almost no configuration, running in seconds. Every change is judged against that first. If something adds setup, options, or steps for the user, it is probably not a fit, however good the idea. We would still rather hear it than not.

## Bugs

Open an issue using the bug report form. It asks for the version, your OS, and how you installed it, because those answer most questions before we have to ask them. If you can, include the exact error text and what you expected to happen instead.

## Ideas

Open an issue using the feature request form before writing code. That way we can talk about whether it fits, and how, before you spend an evening on it.

## Pull requests

- Open an issue first for anything beyond a small fix, so the direction is agreed.
- Keep a pull request to one change. Small and focused is much easier to review and much more likely to merge.
- Match the surrounding code. Comments explain why, not what.
- Run `go build ./...`, `go vet ./...` and `go test -race ./...` before you push. All three must be clean.
- Do not add dependencies without saying why in the pull request.

## Security

Please do not open a public issue for a security problem. See [SECURITY.md](SECURITY.md) for how to report one privately.

## Code of conduct

Be decent to each other. The full version is in [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).
