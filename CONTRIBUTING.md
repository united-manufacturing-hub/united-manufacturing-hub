# Contributing

See also: https://umh.docs.umh.app/docs/development/contribute/

## Two images

umh-core builds as two images:

- The official image includes a private module.
- The open-source image leaves the private module out.

The official image build in CI hides its output, because compiler errors name private files. Maintainers with read access to the private module can rebuild the official image locally to see the error.
