# The FSMv2 dependency map in production

## Channel Provider via the Dependency Map

The transport and communicator workers read their `ChannelProvider` from the
dependency map, under `transport.ChannelProviderKey` and
`communicator.ChannelProviderKey`. Production puts both providers in the map in
`cmd/main.go`:

```go
fsmv2Deps := map[string]any{}
fsmv2config.SetDependency(fsmv2Deps, communicator.ChannelProviderKey, communicator.ChannelProvider(channelAdapter))
fsmv2config.SetDependency(fsmv2Deps, transportWorker.ChannelProviderKey, transportWorker.ChannelProvider(channelAdapter))
```

The supervisor merges its map into every child, so a provider reaches its
worker however deep it sits. `NewTransportWorker` and `NewCommunicatorWorker`
each return an error naming the key when the map holds none. A scenario must
supply a mock provider under the same keys, as "Mocks" in
`examples/SCENARIOS.md` describes.

The push and pull children read the transport worker's channels through its
dependencies, so they share its channels without a provider of their own.

## Cert Handler and Stores via the Dependency Map

| Worker | Key | Value type | `cmd/main.go` sets it |
|---|---|---|---|
| certfetcher | `certfetcher.CertHandlerKey` | `certificatehandler.Handler` | only with the gatekeeper enabled; the comment above that call says why |
| persistence | `persistence.StoreKey` | `storage.TriangularStoreInterface` | always |
| none yet | `secrets.StoreKey` | `secrets.Handle` | always |

Like the channel providers, `NewCertFetcherWorker` and `NewPersistenceWorker`
return an error naming the key when the map holds none.
