// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package secrets holds a store for secret entries, keyed by the hierarchy
// path of the worker that owns them (deps.Identity.HierarchyPath, such as
// "application-fsmv2(application)/communicator-001(communicator)/transport-001(transport)").
//
// cmd/main.go creates one Store per process and puts its Handle in the root
// dependency map under StoreKey. A worker opens its Scope once, in its
// constructor: it looks up the handle and calls ForWorker with its own
// dependencies, which carry its hierarchy path. No worker uses the store yet.
// The transport worker would open its scope like this and keep it on its
// TransportDependencies:
//
//	handle, ok := config.LookupDependency(dependencies, secrets.StoreKey)
//	if !ok {
//		return nil, errors.New("transport: no secrets store in the dependency map")
//	}
//	bd := w.InitBase(identity, logger, stateReader)
//	scope, err := handle.ForWorker(bd)
//	if err != nil {
//		return nil, err
//	}
//	w.BindDeps(NewTransportDependencies(nil, bd, provider, scope))
//
// NewTransportDependencies takes no scope today; this is the planned change.
//
// Get, Put and Delete take a Key (see Key) and act on that entry under the
// scope's path.
//
// Get returns ErrNotFound when nothing is stored under the key yet.
//
// The store keeps a worker's entries until the process exits or the worker
// deletes them. Nothing removes them when the worker is removed or restarted.
// A restart builds a new worker with the same hierarchy path, and that worker
// sees the entries written before the restart. A worker that must not read old
// entries deletes them in its constructor, and deletes them again when it
// stops.
//
// A child reads its parent's entries through the parent's dependencies, not
// through a scope of its own. For example, the push worker
// (workers/transport/push) is a child of the transport worker and sends
// outbound messages with transport's JWT. Its dependency builder gets
// transport's dependencies from register.GlobalDeps. With a Secrets method on
// TransportDependencies, the token would travel like this:
//
//	// AuthenticateAction, in the transport worker
//	err := deps.Secrets().Put(transport.JWTKey, secrets.NewValue(authResp.Token))
//
//	// push worker; transportDeps is the *TransportDependencies from register.GlobalDeps
//	jwt, err := transportDeps.Secrets().Get(transport.JWTKey)
//
// Push only reads the token, so PushDependencies should keep only Get, behind
// an interface such as SecretReader, not the whole Scope.
//
// ForWorker does not check who calls it. Code that passes another worker's
// dependencies, or builds its own PathSource, gets that path's scope.
//
// A Value never prints the string it holds: it prints [secret] and returns
// ErrNotSerializable to JSON and text encoders.
package secrets
