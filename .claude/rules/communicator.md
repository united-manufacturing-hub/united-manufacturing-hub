---
paths:
  - "umh-core/pkg/communicator/**"
  - "umh-core/pkg/models/action_models.go"
---

# Management Console ↔ umh-core

Actions flow from the Management Console to umh-core. Status flows back. Neither direction is ever reversed.

umh-core polls the backend. It calls `GET /v2/instance/pull` for actions and `POST /v2/instance/push` for status. Messages are JSON `UMHMessage` (`pkg/models/action_models.go`).

## Action path

1. The FSMv2 transport pull worker (`pkg/fsmv2/workers/transport/pull/`) fetches messages.
2. The legacy bridge (`pkg/communicator/fsmv2_adapter/`) converts FSMv2 `types.UMHMessage` into `models.UMHMessage`.
3. The router (`pkg/communicator/router/router.go`, `handleAction`) calls `actions.HandleActionMessage` (`pkg/communicator/actions/actions.go`).
4. Each action has its own handler file in `pkg/communicator/actions/`. Handlers that change something write `config.yaml` through the config manager.
5. The FSMs reconcile against the new config, render the benthos config and start the process under S6.

The action type constants are in `pkg/models/action_models.go`. That file is the only complete list.

## Status path

Subscribers (`pkg/communicator/pkg/subscriber/subscribers.go`) build the status message and write it to an outbound channel. The FSMv2 push worker (`pkg/fsmv2/workers/transport/push/`) sends it. When the outbound channel is full, the message is dropped and a Sentry warning is logged (`gatekeeper_outbound_channel_full` or `fsmv2_outbound_channel_full`). The UI then shows stale status while data keeps flowing.
