// Package p2p provides p2p functionality that powers the share exchange protocols used by nubit-node.
// The available protocols are:
//
//   - p2psub : a floodsub-based pubsub protocol that is used to broadcast/subscribe to the event
//     of new EDS in the network to peers.
//
//   - p2pnd: a request/response protocol that is used to request	shares by namespace or namespace data from peers.
//
//   - p2peds: a request/response protocol that is used to request extended data square shares from peers.
//     This protocol exchanges the original data square in between the client and server, and it's up to the
//     receiver to compute the extended data square.
//
// This package also defines a peer manager that is used to manage network peers that can be used to exchange
// shares. The peer manager is primarily responsible for providing peers to request shares from,
// and is primarily used by `getters.P2pGetter` in share/getters/p2p.go.
//
// Find out more about each protocol in their respective sub-packages.
package p2p
